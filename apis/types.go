package apis

import rtv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"

type Reference struct {
	ApiVersion string `json:"apiVersion"`
	Resource   string `json:"resource"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
}

type NamespaceName struct {
	Name      string
	Namespace string
}

type DatabaseConfig struct {
	Spec DatabaseConfigSpec `json:"spec"`
}

type DatabaseConfigSpec struct {
	Username          string                 `json:"username"`
	PasswordSecretRef rtv1.SecretKeySelector `json:"passwordSecretRef"`
}
