package oci

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DemoState 是演示账号的唯一内存状态。
//
// 所有读写都在 OCI 包内短路到这里，进程不会创建任何 SDK 客户端，
// 因此演示账号不可能发出真实 OCI 请求。数据可写：改名、换 IP、创建用户
// 等操作会立刻反映在后续读取上，方便完整试用交互流程。
type DemoState struct {
	mu  sync.Mutex
	seq int

	Regions       []Region
	Compartments  []Compartment
	ADs           []string
	Subnets       []Subnet
	Images        []Image
	Instances     map[string]*Instance
	BootVolumes   map[string]*BootVolume
	Vnics         map[string]*Vnic
	PublicIPs     map[string]*PublicIp
	SecurityLists map[string]*SecurityList
	Users         map[string]*IAMUser
	Groups        map[string]*IAMGroup
	Memberships   map[string]*IAMMembership
	APIKeys       map[string]*IAMApiKey // fingerprint -> key
	Usage         []UsagePoint
	IPCounter     int
}

const (
	demoTenancy   = "ocid1.tenancy.oc1..demo"
	demoAPIUserID = "ocid1.user.oc1..demo-api-user"
	demoCompRoot  = demoTenancy
	demoCompWork  = "ocid1.compartment.oc1..demo-workloads"
	demoVcnID     = "ocid1.vcn.oc1.ap-tokyo-1.demo"
	demoSubnetID  = "ocid1.subnet.oc1.ap-tokyo-1.demo"
)

func (d *DemoState) next(prefix string) string {
	d.seq++
	return fmt.Sprintf("%s.demo-%04d", prefix, d.seq)
}

func newDemoState() *DemoState {
	d := &DemoState{
		Instances:     map[string]*Instance{},
		BootVolumes:   map[string]*BootVolume{},
		Vnics:         map[string]*Vnic{},
		PublicIPs:     map[string]*PublicIp{},
		SecurityLists: map[string]*SecurityList{},
		Users:         map[string]*IAMUser{},
		Groups:        map[string]*IAMGroup{},
		Memberships:   map[string]*IAMMembership{},
		APIKeys:       map[string]*IAMApiKey{},
		IPCounter:     140,
	}
	created := time.Now().UTC().AddDate(0, -2, -12)

	d.Regions = []Region{
		{Key: "NRT", Name: "ap-tokyo-1", Status: "READY", IsHomeRegion: true},
		{Key: "KIX", Name: "ap-osaka-1", Status: "READY", IsHomeRegion: false},
		{Key: "PHX", Name: "us-phoenix-1", Status: "READY", IsHomeRegion: false},
	}
	d.Compartments = []Compartment{
		{ID: demoCompRoot, Name: "(root)", Description: "根区间 / Tenancy"},
		{ID: demoCompWork, Name: "workloads", Description: "演示工作负载"},
	}
	d.ADs = []string{
		"NISh:AP-TOKYO-1-AD-1",
		"NISh:AP-TOKYO-1-AD-2",
		"NISh:AP-TOKYO-1-AD-3",
	}
	d.Subnets = []Subnet{
		{ID: demoSubnetID, DisplayName: "public-subnet", CIDRBlock: "10.0.0.0/24", VcnID: demoVcnID, AvailabilityDomain: "", Region: "ap-tokyo-1"},
		{ID: "ocid1.subnet.oc1.ap-tokyo-1.demo2", DisplayName: "private-subnet", CIDRBlock: "10.0.1.0/24", VcnID: demoVcnID, Region: "ap-tokyo-1"},
	}
	d.Images = []Image{
		{ID: "ocid1.image.oc1.ap-tokyo-1.demo-ubuntu2204", DisplayName: "Canonical Ubuntu 22.04", OperatingSystem: "Canonical Ubuntu", OperatingSystemVersion: "22.04"},
		{ID: "ocid1.image.oc1.ap-tokyo-1.demo-ubuntu2404", DisplayName: "Canonical Ubuntu 24.04", OperatingSystem: "Canonical Ubuntu", OperatingSystemVersion: "24.04"},
		{ID: "ocid1.image.oc1.ap-tokyo-1.demo-ol9", DisplayName: "Oracle Linux 9", OperatingSystem: "Oracle Linux", OperatingSystemVersion: "9"},
	}

	instances := []*Instance{
		{ID: "ocid1.instance.oc1.ap-tokyo-1.demo001", DisplayName: "arm-prod-01", State: "RUNNING", Shape: "VM.Standard.A1.Flex", OCPUs: 4, MemoryGBs: 24, AvailabilityDomain: d.ADs[0], CompartmentID: demoCompWork, Region: "ap-tokyo-1", ImageID: d.Images[0].ID, TimeCreated: &created},
		{ID: "ocid1.instance.oc1.ap-tokyo-1.demo002", DisplayName: "amd-free-01", State: "RUNNING", Shape: "VM.Standard.E2.1.Micro", OCPUs: 1, MemoryGBs: 1, AvailabilityDomain: d.ADs[1], CompartmentID: demoCompRoot, Region: "ap-tokyo-1", ImageID: d.Images[1].ID, TimeCreated: &created},
		{ID: "ocid1.instance.oc1.ap-tokyo-1.demo003", DisplayName: "arm-test-01", State: "STOPPED", Shape: "VM.Standard.A1.Flex", OCPUs: 2, MemoryGBs: 12, AvailabilityDomain: d.ADs[2], CompartmentID: demoCompWork, Region: "ap-tokyo-1", ImageID: d.Images[2].ID, TimeCreated: &created},
	}
	for i, inst := range instances {
		copy := *inst
		d.Instances[inst.ID] = &copy

		vol := &BootVolume{
			ID:          d.next("ocid1.bootvolume.oc1.ap-tokyo-1"),
			DisplayName: inst.DisplayName + " (Boot Volume)",
			SizeGBs:     50, VpusPerGB: 120, State: "AVAILABLE",
			AvailabilityDomain: inst.AvailabilityDomain,
			InstanceID:         inst.ID, CompartmentID: inst.CompartmentID,
		}
		d.BootVolumes[vol.ID] = vol

		vnic := &Vnic{
			ID:            d.next("ocid1.vnic.oc1.ap-tokyo-1"),
			DisplayName:   inst.DisplayName + " primary vNIC",
			HostnameLabel: inst.DisplayName,
			PrivateIP:     fmt.Sprintf("10.0.0.%d", 10+i),
			PublicIP:      fmt.Sprintf("140.238.%d.%d", 20+i, 100+i),
			SubnetID:      demoSubnetID,
			MacAddress:    fmt.Sprintf("02:00:17:0A:%02X:%02X", i+1, 0x41+i),
			State:         "AVAILABLE", InstanceID: inst.ID,
		}
		d.Vnics[vnic.ID] = vnic

		if inst.State == "RUNNING" {
			pip := &PublicIp{
				ID:          d.next("ocid1.publicip.oc1.ap-tokyo-1"),
				IPAddress:   vnic.PublicIP,
				DisplayName: inst.DisplayName + " public IP",
				Lifetime:    "EPHEMERAL",
				PrivateIPID: vnic.ID + "-primary-ip",
				State:       "ASSIGNED", CompartmentID: inst.CompartmentID,
			}
			d.PublicIPs[pip.ID] = pip
		}
	}

	d.SecurityLists["ocid1.securitylist.oc1.ap-tokyo-1.demo"] = &SecurityList{
		ID:          "ocid1.securitylist.oc1.ap-tokyo-1.demo",
		DisplayName: "Default Security List for demo-vcn",
		VcnID:       demoVcnID, CompartmentID: demoCompRoot,
		IngressRules: []SecurityRule{
			{Protocol: "6", Source: "0.0.0.0/0", SourceType: "CIDR_BLOCK", TCPPortMin: intPtrV(22), TCPPortMax: intPtrV(22), Description: "SSH"},
			{Protocol: "6", Source: "0.0.0.0/0", SourceType: "CIDR_BLOCK", TCPPortMin: intPtrV(80), TCPPortMax: intPtrV(80), Description: "HTTP"},
			{Protocol: "6", Source: "0.0.0.0/0", SourceType: "CIDR_BLOCK", TCPPortMin: intPtrV(443), TCPPortMax: intPtrV(443), Description: "HTTPS"},
		},
		EgressRules: []SecurityRule{
			{Protocol: "all", Destination: "0.0.0.0/0", DestinationType: "CIDR_BLOCK", Description: "Allow all egress"},
		},
	}

	d.Users[demoAPIUserID] = &IAMUser{
		ID: demoAPIUserID, Name: "oci-manager-api", Description: "面板自用 API 用户（禁止修改）",
		Email: "ops@example.com", State: "ACTIVE", TimeCreated: created.Format(time.RFC3339),
		CanUseConsolePassword: false, DbUserName: "",
	}
	d.Users["ocid1.user.oc1..demo-admin"] = &IAMUser{
		ID: "ocid1.user.oc1..demo-admin", Name: "admin", Description: "租户管理员",
		Email: "admin@example.com", State: "ACTIVE", TimeCreated: created.Format(time.RFC3339),
		CanUseConsolePassword: true, DbUserName: "",
	}
	d.Users["ocid1.user.oc1..demo-readonly"] = &IAMUser{
		ID: "ocid1.user.oc1..demo-readonly", Name: "auditor", Description: "只读审计账号",
		Email: "audit@example.com", State: "ACTIVE", TimeCreated: created.Format(time.RFC3339),
		CanUseConsolePassword: true, DbUserName: "",
	}

	d.Groups["ocid1.group.oc1..demo-admins"] = &IAMGroup{
		ID: "ocid1.group.oc1..demo-admins", Name: "Administrators", Description: "内置管理员组（禁止变更成员）", State: "ACTIVE", Protected: true,
	}
	d.Groups["ocid1.group.oc1..demo-net"] = &IAMGroup{
		ID: "ocid1.group.oc1..demo-net", Name: "NetworkAdmins", Description: "网络管理组", State: "ACTIVE",
	}
	d.Groups["ocid1.group.oc1..demo-readonly"] = &IAMGroup{
		ID: "ocid1.group.oc1..demo-readonly", Name: "ReadOnly", Description: "只读组", State: "ACTIVE",
	}
	d.Memberships["ocid1.usergroupmembership.oc1..demo1"] = &IAMMembership{
		ID: "ocid1.usergroupmembership.oc1..demo1", UserID: "ocid1.user.oc1..demo-admin", GroupID: "ocid1.group.oc1..demo-admins",
	}
	d.Memberships["ocid1.usergroupmembership.oc1..demo2"] = &IAMMembership{
		ID: "ocid1.usergroupmembership.oc1..demo2", UserID: "ocid1.user.oc1..demo-readonly", GroupID: "ocid1.group.oc1..demo-readonly",
	}
	d.APIKeys["aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99"] = &IAMApiKey{
		UserID: demoAPIUserID, Fingerprint: "aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99",
		State: "ACTIVE", TimeCreated: created.Format(time.RFC3339),
	}
	d.APIKeys["11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00"] = &IAMApiKey{
		UserID: "ocid1.user.oc1..demo-admin", Fingerprint: "11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00",
		State: "ACTIVE", TimeCreated: created.Format(time.RFC3339),
	}

	now := time.Now().UTC()
	d.Usage = []UsagePoint{
		{Service: "COMPUTE", ResourceName: "arm-prod-01", Region: "ap-tokyo-1", Shape: "VM.Standard.A1.Flex", StartTime: now.AddDate(0, 0, -2), EndTime: now.AddDate(0, 0, -1), Cost: 0, Quantity: 24, Currency: "USD"},
		{Service: "BLOCK_STORAGE", ResourceName: "arm-prod-01 (Boot Volume)", Region: "ap-tokyo-1", StartTime: now.AddDate(0, 0, -2), EndTime: now.AddDate(0, 0, -1), Cost: 0, Quantity: 50, Currency: "USD"},
		{Service: "NETWORK", ResourceName: "public-subnet", Region: "ap-tokyo-1", StartTime: now.AddDate(0, 0, -2), EndTime: now.AddDate(0, 0, -1), Cost: 0, Quantity: 1, Currency: "USD"},
	}
	return d
}

func intPtrV(v int) *int { return &v }

/* ------------------------------------------------------------------
   短路：现有 OCI 函数
   ------------------------------------------------------------------ */

func (d *DemoState) listRegions() []Region {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Region(nil), d.Regions...)
}

func (d *DemoState) listAvailabilityDomains() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.ADs...)
}

func (d *DemoState) listCompartments() []Compartment {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Compartment(nil), d.Compartments...)
}

func (d *DemoState) tenancyName() string { return "demo-tenancy" }

func (d *DemoState) listSubnets() []Subnet {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Subnet(nil), d.Subnets...)
}

func (d *DemoState) listImages() []Image {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Image(nil), d.Images...)
}

func (d *DemoState) listInstances(compartmentID string) []Instance {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Instance, 0, len(d.Instances))
	for _, inst := range d.Instances {
		if compartmentID != "" && compartmentID != demoCompRoot && inst.CompartmentID != compartmentID {
			continue
		}
		out = append(out, *inst)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out
}

func (d *DemoState) findInstance(id string) (*Instance, error) {
	inst, ok := d.Instances[id]
	if !ok {
		return nil, fmt.Errorf("演示实例不存在：%s", id)
	}
	return inst, nil
}

func (d *DemoState) instanceAction(id, action string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	inst, err := d.findInstance(id)
	if err != nil {
		return err
	}
	switch strings.ToUpper(action) {
	case "START":
		inst.State = "RUNNING"
	case "STOP", "SOFTSTOP":
		inst.State = "STOPPED"
	case "RESET", "SOFTRESET":
		inst.State = "RUNNING"
	default:
		return fmt.Errorf("不支持的实例操作 %q", action)
	}
	return nil
}

func (d *DemoState) updateInstanceShape(id string, ocpus, memory float32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	inst, err := d.findInstance(id)
	if err != nil {
		return err
	}
	inst.OCPUs, inst.MemoryGBs = ocpus, memory
	return nil
}

func (d *DemoState) updateInstanceDisplayName(id, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	inst, err := d.findInstance(id)
	if err != nil {
		return err
	}
	inst.DisplayName = name
	return nil
}

func (d *DemoState) terminateInstance(id string, preserveBootVolume bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.findInstance(id); err != nil {
		return err
	}
	delete(d.Instances, id)
	for vid, vol := range d.BootVolumes {
		if vol.InstanceID == id {
			if preserveBootVolume {
				vol.InstanceID = ""
				vol.State = "AVAILABLE"
			} else {
				delete(d.BootVolumes, vid)
			}
		}
	}
	for nid, vnic := range d.Vnics {
		if vnic.InstanceID == id {
			for pid, pip := range d.PublicIPs {
				if pip.PrivateIPID == nid+"-primary-ip" {
					delete(d.PublicIPs, pid)
				}
			}
			delete(d.Vnics, nid)
		}
	}
	return nil
}

func (d *DemoState) launchInstance(p LaunchParams) (*Instance, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now().UTC()
	id := d.next("ocid1.instance.oc1.ap-tokyo-1")
	inst := &Instance{
		ID: id, DisplayName: p.DisplayName, State: "PROVISIONING", Shape: p.Shape,
		OCPUs: p.OCPUs, MemoryGBs: p.MemoryGBs, AvailabilityDomain: p.AvailabilityDomain,
		CompartmentID: p.CompartmentID, Region: "ap-tokyo-1", ImageID: p.ImageID, TimeCreated: &now,
	}
	if inst.DisplayName == "" {
		inst.DisplayName = "grabbed-instance"
	}
	d.Instances[id] = inst

	size := p.BootVolumeGBs
	if size <= 0 {
		size = 50
	}
	vol := &BootVolume{
		ID:          d.next("ocid1.bootvolume.oc1.ap-tokyo-1"),
		DisplayName: inst.DisplayName + " (Boot Volume)",
		SizeGBs:     size, VpusPerGB: 120, State: "AVAILABLE",
		AvailabilityDomain: p.AvailabilityDomain, InstanceID: id, CompartmentID: p.CompartmentID,
	}
	d.BootVolumes[vol.ID] = vol

	vnic := &Vnic{
		ID:            d.next("ocid1.vnic.oc1.ap-tokyo-1"),
		DisplayName:   inst.DisplayName + " primary vNIC",
		HostnameLabel: inst.DisplayName,
		PrivateIP:     fmt.Sprintf("10.0.0.%d", 100+d.seq%100),
		SubnetID:      p.SubnetID, MacAddress: "02:00:17:0A:01:99",
		State: "AVAILABLE", InstanceID: id,
	}
	if p.AssignPublicIP {
		d.IPCounter++
		vnic.PublicIP = fmt.Sprintf("140.238.%d.%d", d.IPCounter%200+1, d.IPCounter%250+1)
		pip := &PublicIp{
			ID:        d.next("ocid1.publicip.oc1.ap-tokyo-1"),
			IPAddress: vnic.PublicIP, DisplayName: inst.DisplayName + " public IP",
			Lifetime: "EPHEMERAL", PrivateIPID: vnic.ID + "-primary-ip",
			State: "ASSIGNED", CompartmentID: p.CompartmentID,
		}
		d.PublicIPs[pip.ID] = pip
	}
	d.Vnics[vnic.ID] = vnic
	copy := *inst
	return &copy, nil
}

func (d *DemoState) checkCapacity(q CapacityQuery) []CapacityResult {
	status := CapacityOutOfHostCapacity
	// AD-1 演示为有货，保证抢购任务能走通「预检→创建→成功」完整流程；
	// 其余可用域演示为无货，用于展示过滤与退避日志。
	if strings.Contains(q.AvailabilityDomain, "AD-1") {
		status = CapacityAvailable
	}
	fault := q.FaultDomain
	if fault == "" {
		fault = "FAULT-DOMAIN-1"
	}
	return []CapacityResult{{
		AvailabilityDomain: q.AvailabilityDomain,
		FaultDomain:        fault,
		Shape:              q.Shape,
		OCPUs:              q.OCPUs,
		MemoryGBs:          q.MemoryGBs,
		AvailabilityStatus: status,
		Available:          status == CapacityAvailable,
		AvailableCount:     nil,
		TimeCreated:        time.Now().UTC().Format(time.RFC3339),
	}}
}

func (d *DemoState) queryCost() []UsagePoint {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]UsagePoint(nil), d.Usage...)
}

/* ------------------------------------------------------------------
   短路：资源编辑与网络
   ------------------------------------------------------------------ */

func (d *DemoState) instanceDetail(id string) (InstanceDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	inst, err := d.findInstance(id)
	if err != nil {
		return InstanceDetail{}, err
	}
	out := InstanceDetail{
		Instance:      *inst,
		BootVolumes:   []BootVolume{},
		Vnics:         []Vnic{},
		PublicIPs:     []PublicIp{},
		SecurityLists: []SecurityList{},
	}
	for _, v := range d.BootVolumes {
		if v.InstanceID == id {
			out.BootVolumes = append(out.BootVolumes, *v)
		}
	}
	for _, v := range d.Vnics {
		if v.InstanceID == id {
			out.Vnics = append(out.Vnics, *v)
			for _, p := range d.PublicIPs {
				if p.PrivateIPID == v.ID+"-primary-ip" {
					out.PublicIPs = append(out.PublicIPs, *p)
				}
			}
		}
	}
	for _, s := range d.SecurityLists {
		out.SecurityLists = append(out.SecurityLists, *s)
	}
	sort.Slice(out.BootVolumes, func(i, j int) bool { return out.BootVolumes[i].ID < out.BootVolumes[j].ID })
	sort.Slice(out.Vnics, func(i, j int) bool { return out.Vnics[i].ID < out.Vnics[j].ID })
	return out, nil
}

func (d *DemoState) listBootVolumes(compartmentID, instanceID string) []BootVolume {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]BootVolume, 0, len(d.BootVolumes))
	for _, v := range d.BootVolumes {
		if instanceID != "" && v.InstanceID != instanceID {
			continue
		}
		if compartmentID != "" && compartmentID != demoCompRoot && v.CompartmentID != compartmentID {
			continue
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *DemoState) updateBootVolume(id string, in BootVolumeUpdate) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.BootVolumes[id]
	if !ok {
		return fmt.Errorf("演示引导卷不存在：%s", id)
	}
	if in.DisplayName != nil {
		v.DisplayName = *in.DisplayName
	}
	if in.SizeGBs != nil {
		if *in.SizeGBs < v.SizeGBs {
			return fmt.Errorf("引导卷只能扩容，不能缩小（当前 %d GB）", v.SizeGBs)
		}
		v.SizeGBs = *in.SizeGBs
	}
	if in.VpusPerGB != nil {
		v.VpusPerGB = *in.VpusPerGB
	}
	return nil
}

func (d *DemoState) listVnics(compartmentID, instanceID string) []Vnic {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Vnic, 0, len(d.Vnics))
	for _, v := range d.Vnics {
		if instanceID != "" && v.InstanceID != instanceID {
			continue
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *DemoState) updateVnic(id string, in VnicUpdate) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.Vnics[id]
	if !ok {
		return fmt.Errorf("演示 VNIC 不存在：%s", id)
	}
	if in.DisplayName != nil {
		v.DisplayName = *in.DisplayName
	}
	if in.HostnameLabel != nil {
		v.HostnameLabel = *in.HostnameLabel
	}
	return nil
}

func (d *DemoState) listSecurityLists(compartmentID, vcnID string) []SecurityList {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]SecurityList, 0, len(d.SecurityLists))
	for _, s := range d.SecurityLists {
		if vcnID != "" && s.VcnID != vcnID {
			continue
		}
		copy := *s
		copy.IngressRules = append([]SecurityRule(nil), s.IngressRules...)
		copy.EgressRules = append([]SecurityRule(nil), s.EgressRules...)
		out = append(out, copy)
	}
	return out
}

func (d *DemoState) updateSecurityList(id string, ingress, egress []SecurityRule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.SecurityLists[id]
	if !ok {
		return fmt.Errorf("演示安全列表不存在：%s", id)
	}
	s.IngressRules = append([]SecurityRule(nil), ingress...)
	s.EgressRules = append([]SecurityRule(nil), egress...)
	return nil
}

func (d *DemoState) listPublicIPs(compartmentID string) []PublicIp {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]PublicIp, 0, len(d.PublicIPs))
	for _, p := range d.PublicIPs {
		if compartmentID != "" && compartmentID != demoCompRoot && p.CompartmentID != compartmentID {
			continue
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *DemoState) rotatePublicIP(privateIPID string) (PublicIp, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var (
		targetID   string
		targetVNIC *Vnic
	)
	for id, v := range d.Vnics {
		if id+"-primary-ip" == privateIPID || v.PrivateIP == privateIPID {
			targetID, targetVNIC = id, v
			break
		}
	}
	if targetVNIC == nil {
		return PublicIp{}, fmt.Errorf("未找到对应的私有 IP / VNIC：%s", privateIPID)
	}
	for id, p := range d.PublicIPs {
		if p.PrivateIPID == privateIPID {
			delete(d.PublicIPs, id)
		}
	}
	d.IPCounter++
	newAddr := fmt.Sprintf("140.238.%d.%d", d.IPCounter%200+1, d.IPCounter%250+1)
	targetVNIC.PublicIP = newAddr
	pip := PublicIp{
		ID:        d.next("ocid1.publicip.oc1.ap-tokyo-1"),
		IPAddress: newAddr, DisplayName: "rotated public IP",
		Lifetime: "EPHEMERAL", PrivateIPID: targetID + "-primary-ip",
		State: "ASSIGNED", CompartmentID: demoCompWork,
	}
	d.PublicIPs[pip.ID] = &pip
	return pip, nil
}

/* ------------------------------------------------------------------
   短路：IAM
   ------------------------------------------------------------------ */

func (d *DemoState) iamOverview() IAMOverview {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := IAMOverview{
		Users:            []IAMUser{},
		Groups:           []IAMGroup{},
		Memberships:      []IAMMembership{},
		APIKeys:          []IAMApiKey{},
		ProtectedUserIDs: []string{demoAPIUserID},
		Limits: []IAMLimit{
			{Name: "users", Scope: "TENANCY", Available: 47, Used: 3, Total: 50},
			{Name: "groups", Scope: "TENANCY", Available: 22, Used: 3, Total: 25},
			{Name: "api-keys", Scope: "USER", Available: 2, Used: 1, Total: 3},
		},
	}
	for _, u := range d.Users {
		out.Users = append(out.Users, *u)
	}
	for _, g := range d.Groups {
		out.Groups = append(out.Groups, *g)
	}
	for _, m := range d.Memberships {
		out.Memberships = append(out.Memberships, *m)
	}
	for _, k := range d.APIKeys {
		out.APIKeys = append(out.APIKeys, *k)
	}
	sort.Slice(out.Users, func(i, j int) bool { return out.Users[i].Name < out.Users[j].Name })
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Name < out.Groups[j].Name })
	sort.Slice(out.APIKeys, func(i, j int) bool { return out.APIKeys[i].Fingerprint < out.APIKeys[j].Fingerprint })
	return out
}

func (d *DemoState) createIAMUser(in IAMUserInput) (IAMUser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, u := range d.Users {
		if u.Name == in.Name {
			return IAMUser{}, fmt.Errorf("用户名 %q 已存在", in.Name)
		}
	}
	id := d.next("ocid1.user.oc1..")
	u := IAMUser{
		ID: id, Name: in.Name, Description: in.Description, Email: in.Email,
		State: "ACTIVE", TimeCreated: time.Now().UTC().Format(time.RFC3339),
		CanUseConsolePassword: false,
	}
	d.Users[id] = &u
	return u, nil
}

func (d *DemoState) updateIAMUser(id string, in IAMUserUpdate) (IAMUser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.Users[id]
	if !ok {
		return IAMUser{}, fmt.Errorf("用户不存在：%s", id)
	}
	if in.Description != nil {
		u.Description = *in.Description
	}
	if in.Email != nil {
		u.Email = *in.Email
	}
	if in.Disabled != nil && *in.Disabled {
		u.State = "INACTIVE"
	} else if in.Disabled != nil {
		u.State = "ACTIVE"
	}
	return *u, nil
}

func (d *DemoState) deleteIAMUser(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.Users[id]; !ok {
		return fmt.Errorf("用户不存在：%s", id)
	}
	delete(d.Users, id)
	for mid, m := range d.Memberships {
		if m.UserID == id {
			delete(d.Memberships, mid)
		}
	}
	for fp, k := range d.APIKeys {
		if k.UserID == id {
			delete(d.APIKeys, fp)
		}
	}
	return nil
}

func (d *DemoState) resetIAMPassword(id string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.Users[id]
	if !ok {
		return "", fmt.Errorf("用户不存在：%s", id)
	}
	u.CanUseConsolePassword = true
	return "Demo-0nly-P@ss-8F3k", nil
}

func (d *DemoState) uploadIAMAPIKey(userID, fingerprint string) (IAMApiKey, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.Users[userID]; !ok {
		return IAMApiKey{}, fmt.Errorf("用户不存在：%s", userID)
	}
	key := IAMApiKey{
		UserID: userID, Fingerprint: fingerprint, State: "ACTIVE",
		TimeCreated: time.Now().UTC().Format(time.RFC3339),
	}
	d.APIKeys[fingerprint] = &key
	return key, nil
}

func (d *DemoState) deleteIAMAPIKey(userID, fingerprint string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	k, ok := d.APIKeys[fingerprint]
	if !ok || k.UserID != userID {
		return fmt.Errorf("API Key 不存在：%s", fingerprint)
	}
	delete(d.APIKeys, fingerprint)
	return nil
}

func (d *DemoState) addIAMMember(groupID, userID string) (IAMMembership, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	g, ok := d.Groups[groupID]
	if !ok {
		return IAMMembership{}, fmt.Errorf("组不存在：%s", groupID)
	}
	if g.Protected {
		return IAMMembership{}, fmt.Errorf("禁止变更受保护组 %q 的成员", g.Name)
	}
	if _, ok := d.Users[userID]; !ok {
		return IAMMembership{}, fmt.Errorf("用户不存在：%s", userID)
	}
	for _, m := range d.Memberships {
		if m.GroupID == groupID && m.UserID == userID {
			return IAMMembership{}, fmt.Errorf("用户已在该组中")
		}
	}
	m := IAMMembership{ID: d.next("ocid1.usergroupmembership.oc1.."), GroupID: groupID, UserID: userID}
	d.Memberships[m.ID] = &m
	return m, nil
}

func (d *DemoState) removeIAMMember(groupID, userID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	g, ok := d.Groups[groupID]
	if !ok {
		return fmt.Errorf("组不存在：%s", groupID)
	}
	if g.Protected {
		return fmt.Errorf("禁止变更受保护组 %q 的成员", g.Name)
	}
	for id, m := range d.Memberships {
		if m.GroupID == groupID && m.UserID == userID {
			delete(d.Memberships, id)
			return nil
		}
	}
	return fmt.Errorf("成员关系不存在")
}
