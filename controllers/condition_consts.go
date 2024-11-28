package controllers

const (
	KustomizationReadyCondition = "KustomizationReady"

	WaitingForKustomizationConditionReason = "WaitingForKustomizationCondition"
	WaitingForRepositoryURLReason          = "WaitingForRepositoryURL"
)

const (
	HelmReleaseReadyCondition = "HelmReleaseReady"

	WaitingForClusterReadyReason         = "WaitingForClusterReady"
	WaitingForHelmReleaseConditionReason = "WaitingForHelmReleaseCondition"
)

const (
	DeploymentReferencesReadyCondition = "DeploymentReferencesReady"

	WaitingForDeploymentReadyConditionReason = "WaitingForDeploymentReadyCondition"
	DeploymentReferenceNotReadyReason        = "DeploymentReferenceNotReady"
	UnsupportedDeploymentReferenceKindReason = "UnsupportedDeploymentReferenceKind"
	DeploymentMissingReason                  = "DeploymentMissing"
)

const (
	WaitingForWorkloadTemplateReason    = "WaitingForWorkloadTemplate"
	IncorrectWorkloadTemplateTypeReason = "IncorrectWorkloadTemplateType"
	LoadInstanceFailedReason            = "LoadInstanceFailed"
	BuildInstanceFailedReason           = "BuildInstanceFailed"
	LookupPathFailedReason              = "LookupPathFailed"
	FillPathFailedReason                = "FillPathFailed"
	ValidateFailedReason                = "ValidateFailed"
	FieldsFailedReason                  = "FieldsFailed"
	DecodeFailedReason                  = "DecodeFailed"
	ReconcileFailedReason               = "ReconcileFailed"
)
