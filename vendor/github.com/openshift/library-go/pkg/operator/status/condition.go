package status

import (
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
)

// unionCondition returns a single cluster operator condition that is the union of multiple operator conditions.
//
// defaultConditionStatus indicates whether you want to merge all Falses or merge all Trues.  For instance, Failures merge
// on true, but Available merges on false.  Thing of it like an anti-default.
//
// If interia is non-nil, then resist returning a condition with a status opposite the defaultConditionStatus.
func unionCondition(conditionType string, defaultConditionStatus operatorv1.ConditionStatus, inertia Inertia, allConditions ...operatorv1.OperatorCondition) configv1.ClusterOperatorStatusCondition {
	var oppositeConditionStatus operatorv1.ConditionStatus
	if defaultConditionStatus == operatorv1.ConditionTrue {
		oppositeConditionStatus = operatorv1.ConditionFalse
	} else {
		oppositeConditionStatus = operatorv1.ConditionTrue
	}

	interestingConditions := []operatorv1.OperatorCondition{}
	badConditions := []operatorv1.OperatorCondition{}
	badConditionStatus := operatorv1.ConditionUnknown
	for _, condition := range allConditions {
		if strings.HasSuffix(condition.Type, conditionType) {
			interestingConditions = append(interestingConditions, condition)

			if condition.Status != defaultConditionStatus {
				badConditions = append(badConditions, condition)
				if condition.Status == oppositeConditionStatus {
					badConditionStatus = oppositeConditionStatus
				}
			}
		}
	}

	unionedCondition := operatorv1.OperatorCondition{Type: conditionType, Status: operatorv1.ConditionUnknown}
	if len(interestingConditions) == 0 {
		unionedCondition.Status = operatorv1.ConditionUnknown
		unionedCondition.Reason = "NoData"
		return OperatorConditionToClusterOperatorCondition(unionedCondition)
	}

	var elderBadConditions []operatorv1.OperatorCondition
	if inertia == nil {
		elderBadConditions = badConditions
	} else {
		now := time.Now()
		for _, condition := range badConditions {
			if condition.LastTransitionTime.Time.Before(now.Add(-inertia(condition))) {
				elderBadConditions = append(elderBadConditions, condition)
			}
		}
	}

	if len(elderBadConditions) == 0 {
		unionedCondition.Status = defaultConditionStatus
		unionedCondition.Message = unionMessage(interestingConditions)
		unionedCondition.Reason = "AsExpected"
		unionedCondition.LastTransitionTime = latestTransitionTime(interestingConditions)

		return OperatorConditionToClusterOperatorCondition(unionedCondition)
	}

	// at this point we have bad conditions
	unionedCondition.Status = badConditionStatus
	unionedCondition.Message = unionMessage(badConditions)
	unionedCondition.Reason = unionReason(conditionType, badConditions)
	unionedCondition.LastTransitionTime = latestTransitionTime(badConditions)

	return OperatorConditionToClusterOperatorCondition(unionedCondition)
}

func latestTransitionTime(conditions []operatorv1.OperatorCondition) metav1.Time {
	latestTransitionTime := metav1.Time{}
	for _, condition := range conditions {
		if latestTransitionTime.Before(&condition.LastTransitionTime) {
			latestTransitionTime = condition.LastTransitionTime
		}
	}
	return latestTransitionTime
}

func uniq(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	j := 0
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		s[j] = v
		j++
	}
	return s[:j]
}

func unionMessage(conditions []operatorv1.OperatorCondition) string {
	messages := []string{}
	for _, condition := range conditions {
		if len(condition.Message) == 0 {
			continue
		}
		for _, message := range uniq(strings.Split(condition.Message, "\n")) {
			messages = append(messages, fmt.Sprintf("%s: %s", condition.Type, message))
		}
	}
	return strings.Join(messages, "\n")
}

func unionReason(unionConditionType string, conditions []operatorv1.OperatorCondition) string {
	typeReasons := []string{}
	for _, curr := range conditions {
		currType := curr.Type[:len(curr.Type)-len(unionConditionType)]
		if len(curr.Reason) > 0 {
			typeReasons = append(typeReasons, currType+"_"+curr.Reason)
		} else {
			typeReasons = append(typeReasons, currType)
		}
	}
	sort.Strings(typeReasons)
	return strings.Join(typeReasons, "::")
}
