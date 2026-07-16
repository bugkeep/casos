package server

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			if _, err := crds.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create Traefik CRD %s: %w", desired.Name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("get Traefik CRD %s: %w", desired.Name, err)
		}
		if current.Spec.Group != desired.Spec.Group || current.Spec.Names.Kind != desired.Spec.Names.Kind {
			return fmt.Errorf("Traefik CRD %s is owned by a different API definition", desired.Name)
		}
		if current.Labels["app.kubernetes.io/managed-by"] != "casos" {
			// A CRD installed by the Traefik chart or an operator is already
			// usable; do not overwrite its schema during CasOS bootstrap.
			continue
		}
		updated := current.DeepCopy()
		updated.Labels = mergeStringMap(current.Labels, desired.Labels)
		updated.Spec = desired.Spec
		if equality.Semantic.DeepEqual(current.Labels, updated.Labels) && equality.Semantic.DeepEqual(current.Spec, updated.Spec) {
			continue
		}
		updated.ResourceVersion = current.ResourceVersion
		if _, err := crds.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update Traefik CRD %s: %w", desired.Name, err)
		}
	}
	return nil
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
