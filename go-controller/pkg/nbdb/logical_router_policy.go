// Code generated by "libovsdb.modelgen"
// DO NOT EDIT.

package nbdb

type (
	LogicalRouterPolicyAction = string
)

const (
	LogicalRouterPolicyActionAllow   LogicalRouterPolicyAction = "allow"
	LogicalRouterPolicyActionDrop    LogicalRouterPolicyAction = "drop"
	LogicalRouterPolicyActionReroute LogicalRouterPolicyAction = "reroute"
)

// LogicalRouterPolicy defines an object in Logical_Router_Policy table
type LogicalRouterPolicy struct {
	UUID        string                    `ovsdb:"_uuid"`
	Action      LogicalRouterPolicyAction `ovsdb:"action"`
	ExternalIDs map[string]string         `ovsdb:"external_ids"`
	Match       string                    `ovsdb:"match"`
	Nexthop     []string                  `ovsdb:"nexthop"`
	Nexthops    []string                  `ovsdb:"nexthops"`
	Options     map[string]string         `ovsdb:"options"`
	Priority    int                       `ovsdb:"priority"`
}