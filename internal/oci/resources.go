package oci

import (
	"context"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// UpdateInstanceDisplayName 修改实例的显示名。
// 注意：DisplayName 只是控制台里的名称，不等于机器内 hostname 或 DNS 记录。
func UpdateInstanceDisplayName(ctx context.Context, cs *ClientSet, instanceID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("显示名不能为空")
	}
	if len(name) > 255 {
		return fmt.Errorf("显示名过长")
	}
	if cs.IsDemo() {
		return cs.Demo.updateInstanceDisplayName(instanceID, name)
	}
	_, err := cs.Compute.UpdateInstance(ctx, core.UpdateInstanceRequest{
		InstanceId: &instanceID,
		UpdateInstanceDetails: core.UpdateInstanceDetails{
			DisplayName: &name,
		},
	})
	if err != nil {
		return wrap("修改实例名称失败", err)
	}
	return nil
}

// InstanceDetail 汇总一台实例的关联资源。真实路径全部是只读 List/Get 调用。
func GetInstanceDetail(ctx context.Context, cs *ClientSet, compartmentID, instanceID string) (InstanceDetail, error) {
	if cs.IsDemo() {
		return cs.Demo.instanceDetail(instanceID)
	}
	resp, err := cs.Compute.GetInstance(ctx, core.GetInstanceRequest{InstanceId: &instanceID})
	if err != nil {
		return InstanceDetail{}, wrap("查询实例详情失败", err)
	}
	inst := convertInstance(resp.Instance)
	out := InstanceDetail{
		Instance:      inst,
		BootVolumes:   []BootVolume{},
		Vnics:         []Vnic{},
		PublicIPs:     []PublicIp{},
		SecurityLists: []SecurityList{},
	}

	// 引导卷没有直接的 instanceId 字段，用可用域做近似匹配，
	// 避免为此额外调用需要 mandatory availabilityDomain 的附件接口。
	if volumes, err := ListBootVolumes(ctx, cs, compartmentID); err == nil {
		for _, v := range volumes {
			if v.AvailabilityDomain == inst.AvailabilityDomain || v.AvailabilityDomain == "" {
				v.InstanceID = instanceID
				out.BootVolumes = append(out.BootVolumes, v)
			}
		}
	}

	attachments, err := cs.Compute.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    &instanceID,
		Limit:         intPtr(100),
	})
	if err == nil {
		for _, att := range attachments.Items {
			if att.VnicId == nil {
				continue
			}
			vnicResp, getErr := cs.Network.GetVnic(ctx, core.GetVnicRequest{VnicId: att.VnicId})
			if getErr != nil {
				continue
			}
			out.Vnics = append(out.Vnics, convertVnic(vnicResp.Vnic, instanceID))
		}
	}

	if pips, err := ListPublicIPs(ctx, cs, compartmentID); err == nil {
		out.PublicIPs = pips
	}
	if lists, err := ListSecurityLists(ctx, cs, compartmentID, ""); err == nil {
		out.SecurityLists = lists
	}
	return out, nil
}

// ListBootVolumes 列出区间下的引导卷。
func ListBootVolumes(ctx context.Context, cs *ClientSet, compartmentID string) ([]BootVolume, error) {
	if cs.IsDemo() {
		return cs.Demo.listBootVolumes(compartmentID, ""), nil
	}
	resp, err := cs.BlockStorage.ListBootVolumes(ctx, core.ListBootVolumesRequest{
		CompartmentId: &compartmentID,
		Limit:         intPtr(500),
	})
	if err != nil {
		return nil, wrap("列出引导卷失败", err)
	}
	out := make([]BootVolume, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, convertBootVolume(item, ""))
	}
	return out, nil
}

func convertBootVolume(item core.BootVolume, instanceID string) BootVolume {
	v := BootVolume{State: string(item.LifecycleState), InstanceID: instanceID}
	if item.Id != nil {
		v.ID = *item.Id
	}
	if item.DisplayName != nil {
		v.DisplayName = *item.DisplayName
	}
	if item.AvailabilityDomain != nil {
		v.AvailabilityDomain = *item.AvailabilityDomain
	}
	if item.CompartmentId != nil {
		v.CompartmentID = *item.CompartmentId
	}
	if item.SizeInGBs != nil {
		v.SizeGBs = *item.SizeInGBs
	} else if item.SizeInMBs != nil {
		v.SizeGBs = *item.SizeInMBs / 1024
	}
	if item.VpusPerGB != nil {
		v.VpusPerGB = *item.VpusPerGB
	}
	return v
}

// UpdateBootVolume 修改引导卷名称、容量或 VPU。容量只允许扩容。
func UpdateBootVolume(ctx context.Context, cs *ClientSet, volumeID string, in BootVolumeUpdate) error {
	if cs.IsDemo() {
		return cs.Demo.updateBootVolume(volumeID, in)
	}
	details := core.UpdateBootVolumeDetails{}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if name == "" {
			return fmt.Errorf("显示名不能为空")
		}
		details.DisplayName = &name
	}
	if in.SizeGBs != nil {
		if *in.SizeGBs < 50 || *in.SizeGBs > 32768 {
			return fmt.Errorf("引导卷容量需在 50–32768 GB 之间")
		}
		details.SizeInGBs = in.SizeGBs
	}
	if in.VpusPerGB != nil {
		if *in.VpusPerGB != 10 && *in.VpusPerGB != 20 && (*in.VpusPerGB < 30 || *in.VpusPerGB > 120) {
			return fmt.Errorf("VPU/GB 仅支持 10、20 或 30–120")
		}
		details.VpusPerGB = in.VpusPerGB
	}
	if details.DisplayName == nil && details.SizeInGBs == nil && details.VpusPerGB == nil {
		return fmt.Errorf("没有需要修改的字段")
	}
	_, err := cs.BlockStorage.UpdateBootVolume(ctx, core.UpdateBootVolumeRequest{
		BootVolumeId:            &volumeID,
		UpdateBootVolumeDetails: details,
	})
	if err != nil {
		return wrap("更新引导卷失败", err)
	}
	return nil
}

// ListVnics 通过 VNIC 附件列出实例的网卡。instanceID 为空时列出区间下全部附件。
func ListVnics(ctx context.Context, cs *ClientSet, compartmentID, instanceID string) ([]Vnic, error) {
	if cs.IsDemo() {
		return cs.Demo.listVnics(compartmentID, instanceID), nil
	}
	req := core.ListVnicAttachmentsRequest{CompartmentId: &compartmentID, Limit: intPtr(500)}
	if instanceID != "" {
		req.InstanceId = &instanceID
	}
	resp, err := cs.Compute.ListVnicAttachments(ctx, req)
	if err != nil {
		return nil, wrap("列出 VNIC 附件失败", err)
	}
	out := make([]Vnic, 0, len(resp.Items))
	for _, att := range resp.Items {
		if att.VnicId == nil {
			continue
		}
		vnicResp, err := cs.Network.GetVnic(ctx, core.GetVnicRequest{VnicId: att.VnicId})
		if err != nil {
			continue
		}
		instID := ""
		if att.InstanceId != nil {
			instID = *att.InstanceId
		}
		out = append(out, convertVnic(vnicResp.Vnic, instID))
	}
	return out, nil
}

func convertVnic(item core.Vnic, instanceID string) Vnic {
	v := Vnic{State: string(item.LifecycleState), InstanceID: instanceID}
	if item.Id != nil {
		v.ID = *item.Id
	}
	if item.DisplayName != nil {
		v.DisplayName = *item.DisplayName
	}
	if item.HostnameLabel != nil {
		v.HostnameLabel = *item.HostnameLabel
	}
	if item.PrivateIp != nil {
		v.PrivateIP = *item.PrivateIp
	}
	if item.PublicIp != nil {
		v.PublicIP = *item.PublicIp
	}
	if item.SubnetId != nil {
		v.SubnetID = *item.SubnetId
	}
	if item.MacAddress != nil {
		v.MacAddress = *item.MacAddress
	}
	return v
}

// UpdateVnic 修改 VNIC 显示名与主机名标签。
func UpdateVnic(ctx context.Context, cs *ClientSet, vnicID string, in VnicUpdate) error {
	if cs.IsDemo() {
		return cs.Demo.updateVnic(vnicID, in)
	}
	details := core.UpdateVnicDetails{}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if name == "" {
			return fmt.Errorf("显示名不能为空")
		}
		details.DisplayName = &name
	}
	if in.HostnameLabel != nil {
		label := strings.TrimSpace(*in.HostnameLabel)
		details.HostnameLabel = &label
	}
	if details.DisplayName == nil && details.HostnameLabel == nil {
		return fmt.Errorf("没有需要修改的字段")
	}
	_, err := cs.Network.UpdateVnic(ctx, core.UpdateVnicRequest{
		VnicId:            &vnicID,
		UpdateVnicDetails: details,
	})
	if err != nil {
		return wrap("更新 VNIC 失败", err)
	}
	return nil
}

// ListSecurityLists 列出区间下的安全列表与规则。
func ListSecurityLists(ctx context.Context, cs *ClientSet, compartmentID, vcnID string) ([]SecurityList, error) {
	if cs.IsDemo() {
		return cs.Demo.listSecurityLists(compartmentID, vcnID), nil
	}
	req := core.ListSecurityListsRequest{CompartmentId: &compartmentID, Limit: intPtr(200)}
	if vcnID != "" {
		req.VcnId = &vcnID
	}
	resp, err := cs.Network.ListSecurityLists(ctx, req)
	if err != nil {
		return nil, wrap("列出安全列表失败", err)
	}
	out := make([]SecurityList, 0, len(resp.Items))
	for _, item := range resp.Items {
		list := SecurityList{IngressRules: []SecurityRule{}, EgressRules: []SecurityRule{}}
		if item.Id != nil {
			list.ID = *item.Id
		}
		if item.DisplayName != nil {
			list.DisplayName = *item.DisplayName
		}
		if item.VcnId != nil {
			list.VcnID = *item.VcnId
		}
		if item.CompartmentId != nil {
			list.CompartmentID = *item.CompartmentId
		}
		for _, r := range item.IngressSecurityRules {
			list.IngressRules = append(list.IngressRules, convertIngressRule(r))
		}
		for _, r := range item.EgressSecurityRules {
			list.EgressRules = append(list.EgressRules, convertEgressRule(r))
		}
		out = append(out, list)
	}
	return out, nil
}

func convertIngressRule(r core.IngressSecurityRule) SecurityRule {
	out := SecurityRule{SourceType: string(r.SourceType)}
	if out.SourceType == "" {
		out.SourceType = "CIDR_BLOCK"
	}
	if r.Protocol != nil {
		out.Protocol = *r.Protocol
	}
	if r.Source != nil {
		out.Source = *r.Source
	}
	if r.Description != nil {
		out.Description = *r.Description
	}
	if r.IsStateless != nil {
		out.Stateless = *r.IsStateless
	}
	if r.TcpOptions != nil && r.TcpOptions.DestinationPortRange != nil {
		out.TCPPortMin = copyInt(r.TcpOptions.DestinationPortRange.Min)
		out.TCPPortMax = copyInt(r.TcpOptions.DestinationPortRange.Max)
	}
	if r.UdpOptions != nil && r.UdpOptions.DestinationPortRange != nil {
		out.UDPPortMin = copyInt(r.UdpOptions.DestinationPortRange.Min)
		out.UDPPortMax = copyInt(r.UdpOptions.DestinationPortRange.Max)
	}
	return out
}

func convertEgressRule(r core.EgressSecurityRule) SecurityRule {
	out := SecurityRule{DestinationType: string(r.DestinationType)}
	if out.DestinationType == "" {
		out.DestinationType = "CIDR_BLOCK"
	}
	if r.Protocol != nil {
		out.Protocol = *r.Protocol
	}
	if r.Destination != nil {
		out.Destination = *r.Destination
	}
	if r.Description != nil {
		out.Description = *r.Description
	}
	if r.IsStateless != nil {
		out.Stateless = *r.IsStateless
	}
	if r.TcpOptions != nil && r.TcpOptions.DestinationPortRange != nil {
		out.TCPPortMin = copyInt(r.TcpOptions.DestinationPortRange.Min)
		out.TCPPortMax = copyInt(r.TcpOptions.DestinationPortRange.Max)
	}
	if r.UdpOptions != nil && r.UdpOptions.DestinationPortRange != nil {
		out.UDPPortMin = copyInt(r.UdpOptions.DestinationPortRange.Min)
		out.UDPPortMax = copyInt(r.UdpOptions.DestinationPortRange.Max)
	}
	return out
}

func copyInt(v *int) *int {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}

func portRange(min, max *int) *core.PortRange {
	if min == nil || max == nil {
		return nil
	}
	return &core.PortRange{Min: min, Max: max}
}

// UpdateSecurityList 全量替换安全列表的出入站规则。
func UpdateSecurityList(ctx context.Context, cs *ClientSet, listID string, ingress, egress []SecurityRule) error {
	if cs.IsDemo() {
		return cs.Demo.updateSecurityList(listID, ingress, egress)
	}
	if len(ingress) > 100 || len(egress) > 100 {
		return fmt.Errorf("单组规则不能超过 100 条")
	}
	ingressSDK := make([]core.IngressSecurityRule, 0, len(ingress))
	for i := range ingress {
		r := &ingress[i]
		protocol := r.Protocol
		if protocol == "" {
			protocol = "all"
		}
		source := r.Source
		if source == "" {
			source = "0.0.0.0/0"
		}
		stateless := r.Stateless
		desc := r.Description
		item := core.IngressSecurityRule{
			Protocol:    &protocol,
			Source:      &source,
			IsStateless: &stateless,
			Description: &desc,
			SourceType:  core.IngressSecurityRuleSourceTypeEnum(r.SourceType),
		}
		if item.SourceType == "" {
			item.SourceType = core.IngressSecurityRuleSourceTypeCidrBlock
		}
		if r.TCPPortMin != nil || r.TCPPortMax != nil {
			item.TcpOptions = &core.TcpOptions{DestinationPortRange: portRange(r.TCPPortMin, r.TCPPortMax)}
		}
		if r.UDPPortMin != nil || r.UDPPortMax != nil {
			item.UdpOptions = &core.UdpOptions{DestinationPortRange: portRange(r.UDPPortMin, r.UDPPortMax)}
		}
		ingressSDK = append(ingressSDK, item)
	}
	egressSDK := make([]core.EgressSecurityRule, 0, len(egress))
	for i := range egress {
		r := &egress[i]
		protocol := r.Protocol
		if protocol == "" {
			protocol = "all"
		}
		destination := r.Destination
		if destination == "" {
			destination = "0.0.0.0/0"
		}
		stateless := r.Stateless
		desc := r.Description
		item := core.EgressSecurityRule{
			Protocol:        &protocol,
			Destination:     &destination,
			IsStateless:     &stateless,
			Description:     &desc,
			DestinationType: core.EgressSecurityRuleDestinationTypeEnum(r.DestinationType),
		}
		if item.DestinationType == "" {
			item.DestinationType = core.EgressSecurityRuleDestinationTypeCidrBlock
		}
		if r.TCPPortMin != nil || r.TCPPortMax != nil {
			item.TcpOptions = &core.TcpOptions{DestinationPortRange: portRange(r.TCPPortMin, r.TCPPortMax)}
		}
		if r.UDPPortMin != nil || r.UDPPortMax != nil {
			item.UdpOptions = &core.UdpOptions{DestinationPortRange: portRange(r.UDPPortMin, r.UDPPortMax)}
		}
		egressSDK = append(egressSDK, item)
	}
	_, err := cs.Network.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
		SecurityListId: &listID,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: ingressSDK,
			EgressSecurityRules:  egressSDK,
		},
	})
	if err != nil {
		return wrap("更新安全列表失败", err)
	}
	return nil
}

// ListPublicIPs 列出区间下的公网 IP（REGION 作用域）。
func ListPublicIPs(ctx context.Context, cs *ClientSet, compartmentID string) ([]PublicIp, error) {
	if cs.IsDemo() {
		return cs.Demo.listPublicIPs(compartmentID), nil
	}
	resp, err := cs.Network.ListPublicIps(ctx, core.ListPublicIpsRequest{
		Scope:         core.ListPublicIpsScopeEnum("REGION"),
		CompartmentId: &compartmentID,
		Limit:         intPtr(500),
	})
	if err != nil {
		return nil, wrap("列出公网 IP 失败", err)
	}
	out := make([]PublicIp, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, convertPublicIP(item))
	}
	return out, nil
}

func convertPublicIP(item core.PublicIp) PublicIp {
	p := PublicIp{State: string(item.LifecycleState), Lifetime: string(item.Lifetime)}
	if item.Id != nil {
		p.ID = *item.Id
	}
	if item.IpAddress != nil {
		p.IPAddress = *item.IpAddress
	}
	if item.DisplayName != nil {
		p.DisplayName = *item.DisplayName
	}
	if item.PrivateIpId != nil {
		p.PrivateIPID = *item.PrivateIpId
	}
	if item.CompartmentId != nil {
		p.CompartmentID = *item.CompartmentId
	}
	return p
}

// RotatePublicIP 更换绑定到指定私有 IP 的临时公网 IP。
//
// 实现方式：先删除现有 EPHEMERAL IP 再重新分配。这是 OCI 官方推荐的换 IP 路径，
// Reserved IP 不会被动到。调用方必须自行做冷却、次数上限与二次确认。
func RotatePublicIP(ctx context.Context, cs *ClientSet, privateIPID, compartmentID string) (PublicIp, error) {
	if privateIPID == "" {
		return PublicIp{}, fmt.Errorf("private_ip_id 不能为空")
	}
	if cs.IsDemo() {
		return cs.Demo.rotatePublicIP(privateIPID)
	}
	current, err := cs.Network.GetPublicIpByPrivateIpId(ctx, core.GetPublicIpByPrivateIpIdRequest{
		GetPublicIpByPrivateIpIdDetails: core.GetPublicIpByPrivateIpIdDetails{
			PrivateIpId: &privateIPID,
		},
	})
	if err != nil {
		return PublicIp{}, wrap("查询当前公网 IP 失败", err)
	}
	if current.Id == nil {
		return PublicIp{}, fmt.Errorf("该私有 IP 当前没有绑定公网 IP")
	}
	if string(current.Lifetime) == "RESERVED" {
		return PublicIp{}, fmt.Errorf("该地址是预留公网 IP，请到 OCI 控制台手动解绑，本功能只更换临时 IP")
	}
	if _, err := cs.Network.DeletePublicIp(ctx, core.DeletePublicIpRequest{PublicIpId: current.Id}); err != nil {
		return PublicIp{}, wrap("释放旧公网 IP 失败", err)
	}
	name := "oci-manager rotated public IP"
	created, err := cs.Network.CreatePublicIp(ctx, core.CreatePublicIpRequest{
		CreatePublicIpDetails: core.CreatePublicIpDetails{
			CompartmentId: &compartmentID,
			Lifetime:      core.CreatePublicIpDetailsLifetimeEphemeral,
			PrivateIpId:   &privateIPID,
			DisplayName:   &name,
		},
	})
	if err != nil {
		return PublicIp{}, wrap("分配新公网 IP 失败（旧 IP 已释放）", err)
	}
	return convertPublicIP(created.PublicIp), nil
}
