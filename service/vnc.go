package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"golang.org/x/crypto/ssh"
)

// GenerateSSHKeypair 瞬间生成一对临时 RSA 密钥
func GenerateSSHKeypair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)
	privBlock := pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}
	privatePEM := string(pem.EncodeToMemory(&privBlock))

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	return privatePEM, string(ssh.MarshalAuthorizedKey(publicRsaKey)), nil
}

// CreateVNCConnection 申请通道（带自动清理幽灵通道功能）
func CreateVNCConnection(accountID int, instanceID string) (core.InstanceConsoleConnection, string, error) {
	computeClient, _, _, err := buildClient(accountID)
	if err != nil {
		return core.InstanceConsoleConnection{}, "", err
	}

	// 🚀 第一步：清道夫模式。查找并删除残留的僵尸通道
	getInstReq := core.GetInstanceRequest{InstanceId: common.String(instanceID)}
	instResp, err := computeClient.GetInstance(context.Background(), getInstReq)
	
	if err == nil && instResp.CompartmentId != nil {
		listReq := core.ListInstanceConsoleConnectionsRequest{
			CompartmentId: instResp.CompartmentId,
			InstanceId:    common.String(instanceID),
		}
		listResp, err := computeClient.ListInstanceConsoleConnections(context.Background(), listReq)
		if err == nil {
			cleaned := false
			for _, cc := range listResp.Items {
				// 发现活跃的旧通道，直接发送销毁指令
				if cc.LifecycleState == core.InstanceConsoleConnectionLifecycleStateActive || 
				   cc.LifecycleState == core.InstanceConsoleConnectionLifecycleStateCreating {
					delReq := core.DeleteInstanceConsoleConnectionRequest{
						InstanceConsoleConnectionId: cc.Id,
					}
					_, _ = computeClient.DeleteInstanceConsoleConnection(context.Background(), delReq)
					cleaned = true
				}
			}
			// 如果刚刚删除了旧通道，让程序等 4 秒钟，给甲骨文后台释放资源的时间
			if cleaned {
				time.Sleep(4 * time.Second)
			}
		}
	}

	// 🚀 第二步：正式生成一次性钥匙，申请新通道
	privateKey, publicKey, err := GenerateSSHKeypair()
	if err != nil {
		return core.InstanceConsoleConnection{}, "", fmt.Errorf("生成临时密钥失败: %v", err)
	}

	req := core.CreateInstanceConsoleConnectionRequest{
		CreateInstanceConsoleConnectionDetails: core.CreateInstanceConsoleConnectionDetails{
			InstanceId: common.String(instanceID),
			PublicKey:  common.String(publicKey),
		},
	}

	resp, err := computeClient.CreateInstanceConsoleConnection(context.Background(), req)
	if err != nil {
		return core.InstanceConsoleConnection{}, "", fmt.Errorf("申请 Oracle VNC 通道被拒绝: %v", err)
	}

	return resp.InstanceConsoleConnection, privateKey, nil
}

// DeleteVNCConnection 用户主动关掉网页时的扫尾动作
func DeleteVNCConnection(accountID int, consoleConnectionID string) error {
	computeClient, _, _, err := buildClient(accountID)
	if err != nil {
		return err
	}

	req := core.DeleteInstanceConsoleConnectionRequest{
		InstanceConsoleConnectionId: common.String(consoleConnectionID),
	}
	_, err = computeClient.DeleteInstanceConsoleConnection(context.Background(), req)
	return err
}
