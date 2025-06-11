package kustomization

_name: !=""
_name: string @tag(name)
_tag:  !=""
_tag:  string @tag(tag)

apiVersion: "kustomize.config.k8s.io/v1beta1"
kind:       "Kustomization"
resources: [
	"base",
	"rbac",
	"webhook",
]
images: [
	{
		name:    "dockyards-flux2"
		newName: _name
		newTag:  _tag
	},
]
