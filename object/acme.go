package object

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	acmeAccountSecret    = "casos-acme-account"
	letsEncryptDirectory = "https://acme-v02.api.letsencrypt.org/directory"
)

// challengeTokens holds pending HTTP-01 challenge responses keyed by token.
var challengeTokens sync.Map

func SetACMEChallenge(token, keyAuth string) { challengeTokens.Store(token, keyAuth) }
func GetACMEChallenge(token string) (string, bool) {
	v, ok := challengeTokens.Load(token)
	if !ok {
		return "", false
	}
	return v.(string), true
}
func DeleteACMEChallenge(token string) { challengeTokens.Delete(token) }

// StoreTLSSecret creates or replaces a kubernetes.io/tls Secret with the given PEM content.
func StoreTLSSecret(cfg *rest.Config, namespace, secretName, certPEM, keyPEM string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte(certPEM),
			"tls.key": []byte(keyPEM),
		},
	}
	existing, getErr := client.CoreV1().Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if getErr != nil {
		_, err = client.CoreV1().Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	} else {
		secret.ResourceVersion = existing.ResourceVersion
		_, err = client.CoreV1().Secrets(namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	}
	return err
}

// AttachTLSToIngress updates an Ingress to reference secretName in spec.tls,
// covering all hosts already present in spec.rules.
func AttachTLSToIngress(cfg *rest.Config, namespace, ingressName, secretName string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	ing, err := client.NetworkingV1().Ingresses(namespace).Get(context.Background(), ingressName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	hostSet := map[string]struct{}{}
	for _, r := range ing.Spec.Rules {
		if r.Host != "" {
			hostSet[r.Host] = struct{}{}
		}
	}
	hosts := make([]string, 0, len(hostSet))
	for h := range hostSet {
		hosts = append(hosts, h)
	}
	ing.Spec.TLS = []networkingv1.IngressTLS{{Hosts: hosts, SecretName: secretName}}
	_, err = client.NetworkingV1().Ingresses(namespace).Update(context.Background(), ing, metav1.UpdateOptions{})
	return err
}

// ObtainLECert runs the full ACME HTTP-01 flow for domain, stores the resulting
// certificate as a TLS Secret, and attaches it to ingressName.
//
// casosServiceName / casosServicePort identify the k8s Service that exposes the
// casos server, used to create the temporary challenge Ingress.
func ObtainLECert(cfg *rest.Config, namespace, ingressName, domain, casosServiceName string, casosServicePort int32) error {
	accountKey, err := ensureACMEAccountKey(cfg, namespace)
	if err != nil {
		return fmt.Errorf("acme account key: %w", err)
	}

	acmeClient := &acme.Client{
		Key:          accountKey,
		DirectoryURL: letsEncryptDirectory,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Register (idempotent – error if account already exists is fine).
	_, err = acmeClient.Register(ctx, &acme.Account{}, acme.AcceptTOS)
	if err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		if acmeErr := new(acme.Error); errors.As(err, &acmeErr) && acmeErr.StatusCode == 409 {
			// LE returns 409 for existing accounts; treat as success.
		} else {
			return fmt.Errorf("acme register: %w", err)
		}
	}

	order, err := acmeClient.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return fmt.Errorf("acme authorize order: %w", err)
	}

	for _, authzURL := range order.AuthzURLs {
		authz, err := acmeClient.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("get authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}

		var chal *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "http-01" {
				chal = c
				break
			}
		}
		if chal == nil {
			return fmt.Errorf("no http-01 challenge for %s", authz.Identifier.Value)
		}

		keyAuth, err := acmeClient.HTTP01ChallengeResponse(chal.Token)
		if err != nil {
			return fmt.Errorf("http01 challenge response: %w", err)
		}

		SetACMEChallenge(chal.Token, keyAuth)
		defer DeleteACMEChallenge(chal.Token)

		tempName := ingressName + "-acme-tmp"
		if err := createChallengeIngress(cfg, namespace, tempName, domain, casosServiceName, casosServicePort); err != nil {
			return fmt.Errorf("create challenge ingress: %w", err)
		}
		defer deleteChallengeIngress(cfg, namespace, tempName)

		// Give the Ingress controller time to propagate the route.
		time.Sleep(5 * time.Second)

		if _, err := acmeClient.Accept(ctx, chal); err != nil {
			return fmt.Errorf("accept challenge: %w", err)
		}
		if _, err := acmeClient.WaitAuthorization(ctx, authzURL); err != nil {
			return fmt.Errorf("wait authorization: %w", err)
		}
	}

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate cert key: %w", err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: []string{domain}}, certKey)
	if err != nil {
		return fmt.Errorf("create csr: %w", err)
	}

	der, _, err := acmeClient.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return fmt.Errorf("finalize order: %w", err)
	}

	var certPEM []byte
	for _, d := range der {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: d})...)
	}
	keyBytes, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return fmt.Errorf("marshal cert key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	secretName := ingressName + "-tls"
	if err := StoreTLSSecret(cfg, namespace, secretName, string(certPEM), string(keyPEM)); err != nil {
		return fmt.Errorf("store tls secret: %w", err)
	}
	if err := AttachTLSToIngress(cfg, namespace, ingressName, secretName); err != nil {
		return fmt.Errorf("attach tls: %w", err)
	}
	return nil
}

// ensureACMEAccountKey returns the ACME account key from a cluster Secret,
// creating a new one if it does not exist.
func ensureACMEAccountKey(cfg *rest.Config, namespace string) (*ecdsa.PrivateKey, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	secret, err := client.CoreV1().Secrets(namespace).Get(context.Background(), acmeAccountSecret, metav1.GetOptions{})
	if err == nil {
		if block, _ := pem.Decode(secret.Data["account.key"]); block != nil {
			if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return key, nil
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: acmeAccountSecret, Namespace: namespace},
		Data:       map[string][]byte{"account.key": keyPEM},
	}
	if _, err = client.CoreV1().Secrets(namespace).Create(context.Background(), newSecret, metav1.CreateOptions{}); err != nil {
		// Might already exist due to race; try update.
		if existing, getErr := client.CoreV1().Secrets(namespace).Get(context.Background(), acmeAccountSecret, metav1.GetOptions{}); getErr == nil {
			newSecret.ResourceVersion = existing.ResourceVersion
			_, _ = client.CoreV1().Secrets(namespace).Update(context.Background(), newSecret, metav1.UpdateOptions{})
		}
	}
	return key, nil
}

func createChallengeIngress(cfg *rest.Config, namespace, name, host, serviceName string, servicePort int32) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	pt := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/.well-known/acme-challenge/",
							PathType: &pt,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: serviceName,
									Port: networkingv1.ServiceBackendPort{Number: servicePort},
								},
							},
						}},
					},
				},
			}},
		},
	}
	_, err = client.NetworkingV1().Ingresses(namespace).Create(context.Background(), ing, metav1.CreateOptions{})
	return err
}

func deleteChallengeIngress(cfg *rest.Config, namespace, name string) {
	client, err := newClient(cfg)
	if err != nil {
		return
	}
	_ = client.NetworkingV1().Ingresses(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
