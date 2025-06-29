// Code generated by cue get go. DO NOT EDIT.

//cue:generate cue get go github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// KafkaConnector
#KafkaConnector: {
	metav1.#TypeMeta
	metadata?: metav1.#ObjectMeta @go(ObjectMeta)

	// The specification of the Kafka Connector.
	spec?: null | #KafkaConnectorSpec @go(Spec,*KafkaConnectorSpec)

	// The status of the Kafka Connector.
	status?: null | #KafkaConnectorStatus @go(Status,*KafkaConnectorStatus)
}

// +kubebuilder:object:root=true
// KafkaConnectorList contains a list of instances.
#KafkaConnectorList: {
	metav1.#TypeMeta
	metadata?: metav1.#ListMeta @go(ListMeta)

	// A list of KafkaConnector objects.
	items?: [...#KafkaConnector] @go(Items,[]KafkaConnector)
}

// The specification of the Kafka Connector.
#KafkaConnectorSpec: {
	// Automatic restart of connector and tasks configuration.
	autoRestart?: null | #KafkaConnectorSpecAutoRestart @go(AutoRestart,*KafkaConnectorSpecAutoRestart)

	// The Class for the Kafka Connector.
	class?: null | string @go(Class,*string)

	// The Kafka Connector configuration. The following properties cannot be set:
	// connector.class, tasks.max.
	config?: null | apiextensions.#JSON @go(Config,*apiextensions.JSON)

	// Whether the connector should be paused. Defaults to false.
	pause?: null | bool @go(Pause,*bool)

	// The state the connector should be in. Defaults to running.
	state?: null | #KafkaConnectorSpecState @go(State,*KafkaConnectorSpecState)

	// The maximum number of tasks for the Kafka Connector.
	tasksMax?: null | int32 @go(TasksMax,*int32)
}

// Automatic restart of connector and tasks configuration.
#KafkaConnectorSpecAutoRestart: {
	// Whether automatic restart for failed connectors and tasks should be enabled or
	// disabled.
	enabled?: null | bool @go(Enabled,*bool)

	// The maximum number of connector restarts that the operator will try. If the
	// connector remains in a failed state after reaching this limit, it must be
	// restarted manually by the user. Defaults to an unlimited number of restarts.
	maxRestarts?: null | int32 @go(MaxRestarts,*int32)
}

#KafkaConnectorSpecState: _ // #enumKafkaConnectorSpecState

#enumKafkaConnectorSpecState:
	#KafkaConnectorSpecStatePaused |
	#KafkaConnectorSpecStateRunning |
	#KafkaConnectorSpecStateStopped

#KafkaConnectorSpecStatePaused: #KafkaConnectorSpecState & "paused"

#KafkaConnectorSpecStateRunning: #KafkaConnectorSpecState & "running"

#KafkaConnectorSpecStateStopped: #KafkaConnectorSpecState & "stopped"

// The status of the Kafka Connector.
#KafkaConnectorStatus: {
	// The auto restart status.
	autoRestart?: null | #KafkaConnectorStatusAutoRestart @go(AutoRestart,*KafkaConnectorStatusAutoRestart)

	// List of status conditions.
	conditions?: [...#KafkaConnectorStatusConditionsElem] @go(Conditions,[]KafkaConnectorStatusConditionsElem)

	// The connector status, as reported by the Kafka Connect REST API.
	connectorStatus?: null | apiextensions.#JSON @go(ConnectorStatus,*apiextensions.JSON)

	// The generation of the CRD that was last reconciled by the operator.
	observedGeneration?: null | int32 @go(ObservedGeneration,*int32)

	// The maximum number of tasks for the Kafka Connector.
	tasksMax?: null | int32 @go(TasksMax,*int32)

	// The list of topics used by the Kafka Connector.
	topics?: [...string] @go(Topics,[]string)
}

// The auto restart status.
#KafkaConnectorStatusAutoRestart: {
	// The name of the connector being restarted.
	connectorName?: null | string @go(ConnectorName,*string)

	// The number of times the connector or task is restarted.
	count?: null | int32 @go(Count,*int32)

	// The last time the automatic restart was attempted. The required format is
	// 'yyyy-MM-ddTHH:mm:ssZ' in the UTC time zone.
	lastRestartTimestamp?: null | string @go(LastRestartTimestamp,*string)
}

#KafkaConnectorStatusConditionsElem: {
	// Last time the condition of a type changed from one status to another. The
	// required format is 'yyyy-MM-ddTHH:mm:ssZ', in the UTC time zone.
	lastTransitionTime?: null | string @go(LastTransitionTime,*string)

	// Human-readable message indicating details about the condition's last
	// transition.
	message?: null | string @go(Message,*string)

	// The reason for the condition's last transition (a single word in CamelCase).
	reason?: null | string @go(Reason,*string)

	// The status of the condition, either True, False or Unknown.
	status?: null | string @go(Status,*string)

	// The unique identifier of a condition, used to distinguish between other
	// conditions in the resource.
	type?: null | string @go(Type,*string)
}
