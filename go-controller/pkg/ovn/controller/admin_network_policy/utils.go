package adminnetworkpolicy

import (
	"fmt"
	"strings"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/config"
	libovsdbops "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/libovsdb/ops"
	libovsdbutil "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/libovsdb/util"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/nbdb"
	addressset "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/ovn/address_set"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	anpapi "sigs.k8s.io/network-policy-api/apis/v1alpha1"
)

var ErrorANPPriorityUnsupported = errors.New("OVNK only supports priority ranges 0-99")
var ErrorANPWithDuplicatePriority = errors.New("exists with the same priority")

// getPortProtocol returns the OVN syntax-specific protocol value for a v1.Protocol K8s type
func getPortProtocol(proto v1.Protocol) string {
	var protocol string
	switch proto {
	case v1.ProtocolTCP:
		protocol = "tcp"
	case v1.ProtocolSCTP:
		protocol = "sctp"
	case v1.ProtocolUDP:
		protocol = "udp"
	}
	return protocol
}

// getAdminNetworkPolicyPGName will return the hashed name and provided anp name as the port group name
func getAdminNetworkPolicyPGName(name string, isBanp bool) (hashedPGName, readablePGName string) {
	readablePortGroupName := fmt.Sprintf("ANP:%s", name)
	if isBanp {
		readablePortGroupName = fmt.Sprintf("BANP:%s", name)
	}
	return util.HashForOVN(readablePortGroupName), readablePortGroupName
}

// getANPRuleACLDbIDs will return the dbObjectIDs for a given rule's ACLs
func getANPRuleACLDbIDs(name, gressPrefix, gressIndex, protocol, controller string, isBanp bool) *libovsdbops.DbObjectIDs {
	idType := libovsdbops.ACLAdminNetworkPolicy
	if isBanp {
		idType = libovsdbops.ACLBaselineAdminNetworkPolicy
	}
	return libovsdbops.NewDbObjectIDs(idType, controller, map[libovsdbops.ExternalIDKey]string{
		libovsdbops.ObjectNameKey:      name,
		libovsdbops.PolicyDirectionKey: gressPrefix,
		// gressidx is the unique id for address set within given objectName and gressPrefix
		libovsdbops.GressIdxKey: gressIndex,
		// protocol key
		libovsdbops.PortPolicyProtocolKey: protocol,
	})
}

// GetACLActionForANPRule returns the corresponding OVN ACL action for a given ANP rule action
func GetACLActionForANPRule(action anpapi.AdminNetworkPolicyRuleAction) string {
	var ovnACLAction string
	switch action {
	case anpapi.AdminNetworkPolicyRuleActionAllow:
		ovnACLAction = nbdb.ACLActionAllowRelated
	case anpapi.AdminNetworkPolicyRuleActionDeny:
		ovnACLAction = nbdb.ACLActionDrop
	case anpapi.AdminNetworkPolicyRuleActionPass:
		ovnACLAction = nbdb.ACLActionPass
	default:
		panic(fmt.Sprintf("Failed to build ANP ACL: unknown acl action %s", action))
	}
	return ovnACLAction
}

// GetACLActionForBANPRule returns the corresponding OVN ACL action for a given BANP rule action
func GetACLActionForBANPRule(action anpapi.BaselineAdminNetworkPolicyRuleAction) string {
	var ovnACLAction string
	switch action {
	case anpapi.BaselineAdminNetworkPolicyRuleActionAllow:
		ovnACLAction = nbdb.ACLActionAllowRelated
	case anpapi.BaselineAdminNetworkPolicyRuleActionDeny:
		ovnACLAction = nbdb.ACLActionDrop
	default:
		panic(fmt.Sprintf("Failed to build BANP ACL: unknown acl action %s", action))
	}
	return ovnACLAction
}

// GetANPPeerAddrSetDbIDs will return the dbObjectIDs for a given rule's address-set
func GetANPPeerAddrSetDbIDs(name, gressPrefix, gressIndex, controller string, isBanp bool) *libovsdbops.DbObjectIDs {
	idType := libovsdbops.AddressSetAdminNetworkPolicy
	if isBanp {
		idType = libovsdbops.AddressSetBaselineAdminNetworkPolicy
	}
	return libovsdbops.NewDbObjectIDs(idType, controller, map[libovsdbops.ExternalIDKey]string{
		libovsdbops.ObjectNameKey:      name,
		libovsdbops.PolicyDirectionKey: gressPrefix,
		// gressidx is the unique id for address set within given objectName and gressPrefix
		libovsdbops.GressIdxKey: gressIndex,
	})
}

// constructMatchFromAddressSet returns the L3Match for an ACL constructed from a gressRule
func constructMatchFromAddressSet(gressPrefix string, addrSetIndex *libovsdbops.DbObjectIDs) string {
	hashedAddressSetNameIPv4, hashedAddressSetNameIPv6 := addressset.GetHashNamesForAS(addrSetIndex)
	var direction, match string
	if gressPrefix == string(libovsdbutil.ACLIngress) {
		direction = "src"
	} else {
		direction = "dst"
	}

	switch {
	case config.IPv4Mode && config.IPv6Mode:
		match = fmt.Sprintf("(ip4.%s == $%s || ip6.%s == $%s)", direction, hashedAddressSetNameIPv4, direction, hashedAddressSetNameIPv6)
	case config.IPv4Mode:
		match = fmt.Sprintf("(ip4.%s == $%s)", direction, hashedAddressSetNameIPv4)
	case config.IPv6Mode:
		match = fmt.Sprintf("(ip6.%s == $%s)", direction, hashedAddressSetNameIPv6)
	}

	return fmt.Sprintf("(%s)", match)
}

// TODO(tssurya): https://github.com/ovn-org/ovn-kubernetes/pull/3582 merged and we should port
// some of the common functions to the libovsdbutil package and leverage that.
// For now blatantly copying it so that we can leverage the new indices for ports and merge ANP
// without having to do yet another refactor PR
const (
	// emptyProtocol is used to create ACL for gressPolicy that doesn't have port policies hence no protocols
	emptyProtocol = "None"
)

// for a given ingress/egress rule, captures all the provided port ranges and
// individual ports
type gressPolicyPorts struct {
	portList  []string // list of provided ports as string
	portRange []string // list of provided port ranges in OVN ACL format
}

func getProtocolPortsMap(anpRulePorts []*adminNetworkPolicyPort) map[string]*gressPolicyPorts {
	gressProtoPortsMap := make(map[string]*gressPolicyPorts)
	for _, pp := range anpRulePorts {
		protocol := pp.protocol
		gpp, ok := gressProtoPortsMap[protocol]
		if !ok {
			gpp = &gressPolicyPorts{portList: []string{}, portRange: []string{}}
			gressProtoPortsMap[protocol] = gpp
		}
		if pp.endPort != 0 && pp.endPort != pp.port {
			gpp.portRange = append(gpp.portRange, fmt.Sprintf("%d<=%s.dst<=%d", pp.port, protocol, pp.endPort))
		} else if pp.port != 0 {
			gpp.portList = append(gpp.portList, fmt.Sprintf("%d", pp.port))
		}
	}
	return gressProtoPortsMap
}

func constructMatchFromProtocolPorts(protocol string, ports *gressPolicyPorts) string {
	allL4Matches := []string{}
	if len(ports.portList) > 0 {
		// if there is just one port, then don't use `{}`
		template := "%s.dst==%s"
		if len(ports.portList) > 1 {
			template = "%s.dst=={%s}"
		}
		allL4Matches = append(allL4Matches, fmt.Sprintf(template, protocol, strings.Join(ports.portList, ",")))
	}
	allL4Matches = append(allL4Matches, ports.portRange...)
	l4Match := protocol
	if len(allL4Matches) > 0 {
		template := "%s && %s"
		if len(allL4Matches) > 1 {
			template = "%s && (%s)"
		}
		l4Match = fmt.Sprintf(template, protocol, strings.Join(allL4Matches, " || "))
	}
	return l4Match
}
