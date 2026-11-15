package store

import "strings"

// 审计事件的中文标签集中在这里，供「审计日志」页、主页「最近活动」与
// 实时日志流共用。新增操作时只需在本文件登记一次，避免某处漏映射后
// 又把英文 snake_case 直接暴露给用户。
var auditCategoryLabels = map[string]string{
	"settings": "设置",
	"iam":      "身份与权限",
	"compute":  "计算",
	"storage":  "存储",
	"network":  "网络",
	"auth":     "登录",
	"account":  "账号",
	"task":     "抢购",
}

var auditOperationLabels = map[string]string{
	"demo_mode":             "切换演示模式",
	"ip_rotation_policy":    "更新换 IP 保护策略",
	"iam_settings":          "更新 IAM 设置",
	"iam_credential_import": "导入临时凭证",
	"iam_credential_clear":  "清除临时凭证",
	"guard_block":           "安全护栏拦截",
	"iam_create_user":       "创建 IAM 用户",
	"iam_update_user":       "更新 IAM 用户",
	"iam_delete_user":       "删除 IAM 用户",
	"iam_reset_password":    "重置控制台密码",
	"iam_create_api_key":    "上传 API Key",
	"iam_delete_api_key":    "删除 API Key",
	"iam_add_member":        "加入用户组",
	"iam_remove_member":     "移出用户组",
	"rename_instance":       "重命名实例",
	"update_boot_volume":    "更新引导卷",
	"update_vnic":           "更新 VNIC",
	"update_security_list":  "更新安全列表",
	"rotate_public_ip":      "更换公网 IP",
	"power_task_create":     "创建定时任务",
	"power_task_update":     "更新定时任务",
	"power_task_delete":     "删除定时任务",
	"power_task_toggle":     "启用/停用定时任务",
}

var auditResultLabels = map[string]string{
	"ok":      "成功",
	"success": "成功",
	"updated": "已更新",
	"true":    "已启用",
	"false":   "已关闭",
	"blocked": "已拦截",
	"denied":  "已拒绝",
	"failed":  "失败",
	"error":   "错误",
}

func auditLabel(table map[string]string, v string) string {
	if label, ok := table[v]; ok {
		return label
	}
	// 未登记的枚举退化成空格分隔，但不至于把原始 snake_case 直接丢给展示层。
	return strings.ReplaceAll(v, "_", " ")
}

func AuditCategoryLabel(v string) string  { return auditLabel(auditCategoryLabels, v) }
func AuditOperationLabel(v string) string { return auditLabel(auditOperationLabels, v) }
func AuditResultLabel(v string) string    { return auditLabel(auditResultLabels, v) }
