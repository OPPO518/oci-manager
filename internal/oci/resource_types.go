package oci

// BootVolume 是面向前端的引导卷视图。
type BootVolume struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	SizeGBs            int64  `json:"size_gbs"`
	VpusPerGB          int64  `json:"vpus_per_gb"`
	State              string `json:"state"`
	AvailabilityDomain string `json:"availability_domain"`
	InstanceID         string `json:"instance_id"`
	CompartmentID      string `json:"compartment_id"`
}

// BootVolumeUpdate 是一次引导卷更新请求，字段为空表示不修改。
type BootVolumeUpdate struct {
	DisplayName *string
	SizeGBs     *int64
	VpusPerGB   *int64
}

// Vnic 是面向前端的虚拟网卡视图。
type Vnic struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	HostnameLabel string `json:"hostname_label"`
	PrivateIP     string `json:"private_ip"`
	PublicIP      string `json:"public_ip"`
	SubnetID      string `json:"subnet_id"`
	MacAddress    string `json:"mac_address"`
	State         string `json:"state"`
	InstanceID    string `json:"instance_id"`
}

// VnicUpdate 是一次 VNIC 更新请求。
type VnicUpdate struct {
	DisplayName   *string
	HostnameLabel *string
}

// SecurityRule 是与 SDK 解耦的安全规则视图，前端可直接编辑。
type SecurityRule struct {
	Protocol        string `json:"protocol"`
	Source          string `json:"source"`
	SourceType      string `json:"source_type"`
	Destination     string `json:"destination"`
	DestinationType string `json:"destination_type"`
	TCPPortMin      *int   `json:"tcp_port_min,omitempty"`
	TCPPortMax      *int   `json:"tcp_port_max,omitempty"`
	UDPPortMin      *int   `json:"udp_port_min,omitempty"`
	UDPPortMax      *int   `json:"udp_port_max,omitempty"`
	Description     string `json:"description"`
	Stateless       bool   `json:"stateless"`
}

// SecurityList 描述一个 VCN 安全列表及其出入站规则。
type SecurityList struct {
	ID            string         `json:"id"`
	DisplayName   string         `json:"display_name"`
	VcnID         string         `json:"vcn_id"`
	IngressRules  []SecurityRule `json:"ingress_rules"`
	EgressRules   []SecurityRule `json:"egress_rules"`
	CompartmentID string         `json:"compartment_id,omitempty"`
}

// PublicIp 是面向前端的公网 IP 视图。
type PublicIp struct {
	ID            string `json:"id"`
	IPAddress     string `json:"ip_address"`
	DisplayName   string `json:"display_name"`
	Lifetime      string `json:"lifetime"`
	PrivateIPID   string `json:"private_ip_id"`
	State         string `json:"state"`
	CompartmentID string `json:"compartment_id"`
}

// InstanceDetail 聚合一台实例的关联资源，供详情抽屉展示。
type InstanceDetail struct {
	Instance      Instance       `json:"instance"`
	BootVolumes   []BootVolume   `json:"boot_volumes"`
	Vnics         []Vnic         `json:"vnics"`
	PublicIPs     []PublicIp     `json:"public_ips"`
	SecurityLists []SecurityList `json:"security_lists"`
}
