package apis

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
	Username          string            `json:"username"`
	PasswordSecretRef SecretKeySelector `json:"passwordSecretRef"`
}

// A SecretKeySelector is a reference to a secret key in an arbitrary namespace.
type SecretKeySelector struct {
	// Name of the referenced object.
	Name string `json:"name"`

	// Namespace of the referenced object.
	Namespace string `json:"namespace"`

	// The key to select.
	Key string `json:"key"`
}

// DeepCopy copy the receiver, creates a new SecretKeySelector.
func (in *SecretKeySelector) DeepCopy() *SecretKeySelector {
	if in == nil {
		return nil
	}
	out := new(SecretKeySelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copy the receiver, writes into out. in must be non-nil.
func (in *SecretKeySelector) DeepCopyInto(out *SecretKeySelector) {
	*out = *in
}
