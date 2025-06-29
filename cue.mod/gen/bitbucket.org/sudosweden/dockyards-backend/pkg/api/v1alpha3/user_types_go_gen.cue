// Code generated by cue get go. DO NOT EDIT.

//cue:generate cue get go bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3

package v1alpha3

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

#UserKind: "User"

#UserSpec: {
	email:        string                  @go(Email)
	displayName?: string                  @go(DisplayName)
	password:     string                  @go(Password)
	phone?:       string                  @go(Phone)
	duration?:    null | metav1.#Duration @go(Duration,*metav1.Duration)
}

#UserStatus: {
	conditions?: [...metav1.#Condition] @go(Conditions,[]metav1.Condition)
	expirationTimestamp?: null | metav1.#Time @go(ExpirationTimestamp,*metav1.Time)
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="UID",type=string,priority=1,JSONPath=".metadata.uid"
// +kubebuilder:printcolumn:name="Email",type=string,JSONPath=".spec.email"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=".spec.duration"
#User: {
	metav1.#TypeMeta
	metadata?: metav1.#ObjectMeta @go(ObjectMeta)
	spec?:     #UserSpec          @go(Spec)
	status?:   #UserStatus        @go(Status)
}

// +kubebuilder:object:root=true
#UserList: {
	metav1.#TypeMeta
	metadata?: metav1.#ListMeta @go(ListMeta)
	items: [...#User] @go(Items,[]User)
}
