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
