package server

import (
	"context"
	"fmt"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const traefikCRDEstablishedTimeout = 30 * time.Second

type traefikCRD struct {
	plural string
	kind   string
}

var traefikCRDs = []traefikCRD{
	{plural: "ingressroutes", kind: "IngressRoute"},
	{plural: "ingressroutetcps", kind: "IngressRouteTCP"},
	{plural: "ingressrouteudps", kind: "IngressRouteUDP"},
	{plural: "middlewares", kind: "Middleware"},
	{plural: "middlewaretcps", kind: "MiddlewareTCP"},
	{plural: "traefikservices", kind: "TraefikService"},
	{plural: "tlsoptions", kind: "TLSOption"},
	{plural: "tlsstores", kind: "TLSStore"},
	{plural: "serverstransports", kind: "ServersTransport"},
	{plural: "serverstransporttcps", kind: "ServersTransportTCP"},
}

func ensureTraefikCRDs(ctx context.Context, client apiextensionsclient.Interface) error {
	crds := client.ApiextensionsV1().CustomResourceDefinitions()
	for _, definition := range traefikCRDs {
		desired := buildTraefikCRD(definition)
		current, err := crds.Get(ctx, desired.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			current, err = crds.Create(ctx, desired, metav1.CreateOptions{})
			if apierrors.IsAlreadyExists(err) {
				current, err = crds.Get(ctx, desired.Name, metav1.GetOptions{})
			}
			if err != nil {
				return fmt.Errorf("create Traefik CRD %s: %w", desired.Name, err)
			}
		} else if err != nil {
			return fmt.Errorf("get Traefik CRD %s: %w", desired.Name, err)
		}
		if !traefikCRDCompatible(current, desired) {
			return fmt.Errorf("Traefik CRD %s is incompatible with the required API definition", desired.Name)
		}
		if current.Labels["app.kubernetes.io/managed-by"] == "casos" {
			updated := current.DeepCopy()
			updated.Labels = mergeStringMap(current.Labels, desired.Labels)
			if !equality.Semantic.DeepEqual(current.Labels, updated.Labels) {
				updated.ResourceVersion = current.ResourceVersion
				if _, err := crds.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
					return fmt.Errorf("update Traefik CRD %s labels: %w", desired.Name, err)
				}
			}
		}
		if err := waitForTraefikCRDEstablished(ctx, client, desired.Name); err != nil {
			return err
		}
	}
	return nil
}

func traefikCRDCompatible(current, desired *apiextensionsv1.CustomResourceDefinition) bool {
	if current == nil || desired == nil || current.Spec.Group != desired.Spec.Group ||
		current.Spec.Names.Plural != desired.Spec.Names.Plural ||
		current.Spec.Names.Singular != desired.Spec.Names.Singular ||
		current.Spec.Names.Kind != desired.Spec.Names.Kind ||
		current.Spec.Names.ListKind != desired.Spec.Names.ListKind ||
		current.Spec.Scope != desired.Spec.Scope {
		return false
	}
	for _, version := range current.Spec.Versions {
		if version.Name != "v1alpha1" || !version.Served || version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
			continue
		}
		if version.Schema.OpenAPIV3Schema.Properties == nil {
			return false
		}
		if _, ok := version.Schema.OpenAPIV3Schema.Properties["spec"]; !ok {
			return false
		}
		return true
	}
	return false
}

func waitForTraefikCRDEstablished(ctx context.Context, client apiextensionsclient.Interface, name string) error {
	deadline := time.NewTimer(traefikCRDEstablishedTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("get Traefik CRD %s status: %w", name, err)
		}
		if err == nil {
			for _, condition := range current.Status.Conditions {
				if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Traefik CRD %s to be established: %w", name, ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for Traefik CRD %s to be established", name)
		case <-ticker.C:
		}
	}
}

func buildTraefikCRD(definition traefikCRD) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: definition.plural + ".traefik.io",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "traefik",
				"app.kubernetes.io/managed-by": "casos",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "traefik.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   definition.plural,
				Singular: definition.plural[:len(definition.plural)-1],
				Kind:     definition.kind,
				ListKind: definition.kind + "List",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1alpha1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec":   {Type: "object", XPreserveUnknownFields: ptr(true)},
							"status": {Type: "object", XPreserveUnknownFields: ptr(true)},
						},
					},
				},
			}},
		},
	}
}
