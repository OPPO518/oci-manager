package oci

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/identity"
)

// IAMOverview 拉取身份与权限页所需的只读快照。
// 全部是 List 类调用，不产生任何写操作；IAM 资源是 tenancy 级，
// CompartmentId 一律传根区间。
func GetIAMOverview(ctx context.Context, cs *ClientSet, compartmentID, protectedUserID, mode, homeRegion string) (IAMOverview, error) {
	if cs.IsDemo() {
		out := cs.Demo.iamOverview()
		out.Mode = mode
		out.HomeRegion = homeRegion
		return out, nil
	}
	out := IAMOverview{
		Mode:             mode,
		HomeRegion:       homeRegion,
		Users:            []IAMUser{},
		Groups:           []IAMGroup{},
		Memberships:      []IAMMembership{},
		APIKeys:          []IAMApiKey{},
		Limits:           []IAMLimit{},
		ProtectedUserIDs: []string{},
	}
	if protectedUserID != "" {
		out.ProtectedUserIDs = append(out.ProtectedUserIDs, protectedUserID)
	}

	users, err := cs.Identity.ListUsers(ctx, identity.ListUsersRequest{
		CompartmentId: &compartmentID, Limit: intPtr(500),
	})
	if err != nil {
		return out, wrap("列出 IAM 用户失败", err)
	}
	for _, u := range users.Items {
		item := IAMUser{}
		if u.Id != nil {
			item.ID = *u.Id
		}
		if u.Name != nil {
			item.Name = *u.Name
		}
		if u.Description != nil {
			item.Description = *u.Description
		}
		if u.Email != nil {
			item.Email = *u.Email
		}
		if u.DbUserName != nil {
			item.DbUserName = *u.DbUserName
		}
		if u.TimeCreated != nil {
			item.TimeCreated = u.TimeCreated.Time.Format(time.RFC3339)
		}
		item.State = string(u.LifecycleState)
		out.Users = append(out.Users, item)

		keys, keyErr := ListIAMAPIKeys(ctx, cs, item.ID)
		if keyErr == nil {
			out.APIKeys = append(out.APIKeys, keys...)
		}
	}

	groups, err := cs.Identity.ListGroups(ctx, identity.ListGroupsRequest{
		CompartmentId: &compartmentID, Limit: intPtr(500),
	})
	if err != nil {
		return out, wrap("列出 IAM 组失败", err)
	}
	for _, g := range groups.Items {
		item := IAMGroup{}
		if g.Id != nil {
			item.ID = *g.Id
		}
		if g.Name != nil {
			item.Name = *g.Name
		}
		if g.Description != nil {
			item.Description = *g.Description
		}
		item.State = string(g.LifecycleState)
		item.Protected = strings.EqualFold(item.Name, "Administrators")
		out.Groups = append(out.Groups, item)
	}

	memberships, err := cs.Identity.ListUserGroupMemberships(ctx, identity.ListUserGroupMembershipsRequest{
		CompartmentId: &compartmentID, Limit: intPtr(500),
	})
	if err != nil {
		return out, wrap("列出组成员关系失败", err)
	}
	for _, m := range memberships.Items {
		item := IAMMembership{}
		if m.Id != nil {
			item.ID = *m.Id
		}
		if m.UserId != nil {
			item.UserID = *m.UserId
		}
		if m.GroupId != nil {
			item.GroupID = *m.GroupId
		}
		out.Memberships = append(out.Memberships, item)
	}
	return out, nil
}

// GetIAMGroupName 查询组名，供服务端禁止清单使用（不依赖前端传来的名称）。
func GetIAMGroupName(ctx context.Context, cs *ClientSet, groupID string) (string, error) {
	if cs.IsDemo() {
		cs.Demo.mu.Lock()
		defer cs.Demo.mu.Unlock()
		if g, ok := cs.Demo.Groups[groupID]; ok {
			return g.Name, nil
		}
		return "", fmt.Errorf("组不存在：%s", groupID)
	}
	resp, err := cs.Identity.GetGroup(ctx, identity.GetGroupRequest{GroupId: &groupID})
	if err != nil {
		return "", wrap("查询 IAM 组失败", err)
	}
	if resp.Name == nil {
		return "", fmt.Errorf("组不存在：%s", groupID)
	}
	return *resp.Name, nil
}

// ListIAMAPIKeys 列出某个用户的 API 签名公钥。
func ListIAMAPIKeys(ctx context.Context, cs *ClientSet, userID string) ([]IAMApiKey, error) {
	if cs.IsDemo() {
		cs.Demo.mu.Lock()
		defer cs.Demo.mu.Unlock()
		out := []IAMApiKey{}
		for _, k := range cs.Demo.APIKeys {
			if k.UserID == userID {
				out = append(out, *k)
			}
		}
		return out, nil
	}
	resp, err := cs.Identity.ListApiKeys(ctx, identity.ListApiKeysRequest{UserId: &userID})
	if err != nil {
		return nil, wrap("列出 API Key 失败", err)
	}
	out := make([]IAMApiKey, 0, len(resp.Items))
	for _, k := range resp.Items {
		item := IAMApiKey{UserID: userID, State: string(k.LifecycleState)}
		if k.Fingerprint != nil {
			item.Fingerprint = *k.Fingerprint
		}
		if k.TimeCreated != nil {
			item.TimeCreated = k.TimeCreated.Time.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out, nil
}

// CreateIAMUser 创建 IAM 用户。name 创建后不可修改。
func CreateIAMUser(ctx context.Context, cs *ClientSet, compartmentID string, in IAMUserInput) (IAMUser, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return IAMUser{}, fmt.Errorf("用户名不能为空")
	}
	if cs.IsDemo() {
		return cs.Demo.createIAMUser(in)
	}
	desc := in.Description
	if desc == "" {
		desc = "由 OCI 管理器创建"
	}
	details := identity.CreateUserDetails{
		CompartmentId: &compartmentID,
		Name:          &name,
		Description:   &desc,
	}
	email := strings.TrimSpace(in.Email)
	if email != "" {
		details.Email = &email
	}
	resp, err := cs.Identity.CreateUser(ctx, identity.CreateUserRequest{CreateUserDetails: details})
	if err != nil {
		return IAMUser{}, wrap("创建 IAM 用户失败", err)
	}
	return convertIAMUser(resp.User), nil
}

func convertIAMUser(u identity.User) IAMUser {
	out := IAMUser{State: string(u.LifecycleState)}
	if u.Id != nil {
		out.ID = *u.Id
	}
	if u.Name != nil {
		out.Name = *u.Name
	}
	if u.Description != nil {
		out.Description = *u.Description
	}
	if u.Email != nil {
		out.Email = *u.Email
	}
	if u.DbUserName != nil {
		out.DbUserName = *u.DbUserName
	}
	if u.TimeCreated != nil {
		out.TimeCreated = u.TimeCreated.Time.Format(time.RFC3339)
	}
	return out
}

// UpdateIAMUser 修改用户描述与邮箱。经典 IAM 不提供通过 UpdateUser 停用用户的字段，
// 传入 Disabled 会返回明确错误，避免静默误导。
func UpdateIAMUser(ctx context.Context, cs *ClientSet, userID string, in IAMUserUpdate) (IAMUser, error) {
	if cs.IsDemo() {
		return cs.Demo.updateIAMUser(userID, in)
	}
	if in.Disabled != nil {
		return IAMUser{}, fmt.Errorf("经典 IAM 的 UpdateUser 不支持停用用户，请到 OCI 控制台操作或删除该用户")
	}
	details := identity.UpdateUserDetails{}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		details.Description = &desc
	}
	if in.Email != nil {
		email := strings.TrimSpace(*in.Email)
		details.Email = &email
	}
	if details.Description == nil && details.Email == nil {
		return IAMUser{}, fmt.Errorf("没有需要修改的字段")
	}
	resp, err := cs.Identity.UpdateUser(ctx, identity.UpdateUserRequest{
		UserId:            &userID,
		UpdateUserDetails: details,
	})
	if err != nil {
		return IAMUser{}, wrap("更新 IAM 用户失败", err)
	}
	return convertIAMUser(resp.User), nil
}

// DeleteIAMUser 删除用户。调用方必须做二次确认与审计。
func DeleteIAMUser(ctx context.Context, cs *ClientSet, userID string) error {
	if cs.IsDemo() {
		return cs.Demo.deleteIAMUser(userID)
	}
	_, err := cs.Identity.DeleteUser(ctx, identity.DeleteUserRequest{UserId: &userID})
	if err != nil {
		return wrap("删除 IAM 用户失败", err)
	}
	return nil
}

// ResetIAMConsolePassword 重置控制台密码。返回的一次性密码绝不入库、不写日志。
func ResetIAMConsolePassword(ctx context.Context, cs *ClientSet, userID string) (string, error) {
	if cs.IsDemo() {
		return cs.Demo.resetIAMPassword(userID)
	}
	resp, err := cs.Identity.CreateOrResetUIPassword(ctx, identity.CreateOrResetUIPasswordRequest{UserId: &userID})
	if err != nil {
		return "", wrap("重置控制台密码失败", err)
	}
	if resp.Password == nil {
		return "", fmt.Errorf("OCI 未返回一次性密码")
	}
	return *resp.Password, nil
}

// UploadIAMAPIKey 上传 API 签名公钥（PEM 公钥，不是私钥）。
func UploadIAMAPIKey(ctx context.Context, cs *ClientSet, userID, publicKeyPEM string) (IAMApiKey, error) {
	pem := strings.TrimSpace(publicKeyPEM)
	if pem == "" || !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		return IAMApiKey{}, fmt.Errorf("请提供 PEM 格式的 RSA 公钥（BEGIN PUBLIC KEY）")
	}
	if cs.IsDemo() {
		fp := fmt.Sprintf("demo:%02x:%02x", len(pem), int(time.Now().UnixNano()%255))
		return cs.Demo.uploadIAMAPIKey(userID, fp)
	}
	resp, err := cs.Identity.UploadApiKey(ctx, identity.UploadApiKeyRequest{
		UserId:              &userID,
		CreateApiKeyDetails: identity.CreateApiKeyDetails{Key: &pem},
	})
	if err != nil {
		return IAMApiKey{}, wrap("上传 API Key 失败", err)
	}
	out := IAMApiKey{UserID: userID, State: string(resp.LifecycleState)}
	if resp.Fingerprint != nil {
		out.Fingerprint = *resp.Fingerprint
	}
	if resp.TimeCreated != nil {
		out.TimeCreated = resp.TimeCreated.Time.Format(time.RFC3339)
	}
	return out, nil
}

// DeleteIAMAPIKey 删除指定用户的 API 签名公钥。
func DeleteIAMAPIKey(ctx context.Context, cs *ClientSet, userID, fingerprint string) error {
	if cs.IsDemo() {
		return cs.Demo.deleteIAMAPIKey(userID, fingerprint)
	}
	_, err := cs.Identity.DeleteApiKey(ctx, identity.DeleteApiKeyRequest{
		UserId:      &userID,
		Fingerprint: &fingerprint,
	})
	if err != nil {
		return wrap("删除 API Key 失败", err)
	}
	return nil
}

// AddIAMGroupMember 把用户加入组。受保护组的拦截由调用方（服务端禁止清单）负责。
func AddIAMGroupMember(ctx context.Context, cs *ClientSet, groupID, userID string) (IAMMembership, error) {
	if cs.IsDemo() {
		return cs.Demo.addIAMMember(groupID, userID)
	}
	resp, err := cs.Identity.AddUserToGroup(ctx, identity.AddUserToGroupRequest{
		AddUserToGroupDetails: identity.AddUserToGroupDetails{
			GroupId: &groupID,
			UserId:  &userID,
		},
	})
	if err != nil {
		return IAMMembership{}, wrap("加入组失败", err)
	}
	out := IAMMembership{GroupID: groupID, UserID: userID}
	if resp.Id != nil {
		out.ID = *resp.Id
	}
	return out, nil
}

// RemoveIAMGroupMember 把用户移出组。需要先按 group+user 查到 membership ID。
func RemoveIAMGroupMember(ctx context.Context, cs *ClientSet, compartmentID, groupID, userID string) error {
	if cs.IsDemo() {
		return cs.Demo.removeIAMMember(groupID, userID)
	}
	resp, err := cs.Identity.ListUserGroupMemberships(ctx, identity.ListUserGroupMembershipsRequest{
		CompartmentId: &compartmentID,
		GroupId:       &groupID,
		UserId:        &userID,
		Limit:         intPtr(100),
	})
	if err != nil {
		return wrap("查询组成员关系失败", err)
	}
	if len(resp.Items) == 0 || resp.Items[0].Id == nil {
		return fmt.Errorf("成员关系不存在")
	}
	_, err = cs.Identity.RemoveUserFromGroup(ctx, identity.RemoveUserFromGroupRequest{
		UserGroupMembershipId: resp.Items[0].Id,
	})
	if err != nil {
		return wrap("移出组失败", err)
	}
	return nil
}
