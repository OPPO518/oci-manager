package oci

// IAMUser 是 OCI 租户内的用户视图（经典 IAM）。
type IAMUser struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	Email                 string `json:"email"`
	State                 string `json:"state"`
	TimeCreated           string `json:"time_created"`
	CanUseConsolePassword bool   `json:"can_use_console_password"`
	DbUserName            string `json:"db_user_name"`
}

// IAMGroup 是用户组视图。Protected 为 true 表示服务端禁止变更其成员。
type IAMGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`
	Protected   bool   `json:"protected"`
}

// IAMMembership 是用户与组的关联。
type IAMMembership struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id"`
	GroupID string `json:"group_id"`
}

// IAMApiKey 是 API 签名公钥记录。KeyValue 只在演示数据中占位，真实环境不回显。
type IAMApiKey struct {
	UserID      string `json:"user_id"`
	Fingerprint string `json:"fingerprint"`
	State       string `json:"state"`
	TimeCreated string `json:"time_created"`
	KeyValue    string `json:"key_value,omitempty"`
}

// IAMLimit 是 IAM 服务限额视图。
type IAMLimit struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	Available int64  `json:"available"`
	Used      int64  `json:"used"`
	Total     int64  `json:"total"`
}

// IAMOverview 是身份与权限页的一次性快照。
type IAMOverview struct {
	Mode             string          `json:"mode"`
	HomeRegion       string          `json:"home_region"`
	Users            []IAMUser       `json:"users"`
	Groups           []IAMGroup      `json:"groups"`
	Memberships      []IAMMembership `json:"memberships"`
	APIKeys          []IAMApiKey     `json:"api_keys"`
	Limits           []IAMLimit      `json:"limits"`
	ProtectedUserIDs []string        `json:"protected_user_ids"`
	ReadOnly         bool            `json:"read_only"`
	Hint             string          `json:"hint"`
}

// IAMUserInput 是创建用户的参数。
type IAMUserInput struct {
	Name        string
	Description string
	Email       string
}

// IAMUserUpdate 是更新用户的参数，指针为 nil 表示不修改。
type IAMUserUpdate struct {
	Description *string
	Email       *string
	Disabled    *bool
}
