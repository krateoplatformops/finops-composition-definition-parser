package secrets

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	types "finops-composition-definition-parser/apis"
)

func Get(ctx context.Context, rc *rest.Config, sel *types.SecretKeySelector) (*corev1.Secret, error) {
	rc.GroupVersion = &corev1.SchemeGroupVersion
	rc.APIPath = "/api"
	rc.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	cli, err := rest.RESTClientFor(rc)
	if err != nil {
		return nil, err
	}

	res := &corev1.Secret{}
	err = cli.Get().
		Resource("secrets").
		Namespace(sel.Namespace).Name(sel.Name).
		Do(ctx).
		Into(res)

	return res, err
}
