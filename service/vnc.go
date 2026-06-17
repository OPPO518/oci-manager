package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"golang.org/x/crypto/ssh"
)

// GenerateSSHKeypair 瞬间生成一对 2048 位的临时 RSA 密钥（用完即焚）
func GenerateSSHKeypair() (string, string, error) {
	// 1. 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	// 将私钥转成 PEM 格式字符串（Go 留着自己用）
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}
	privatePEM := string(pem.EncodeToMemory(&privBlock))

	// 2. 生成对应的公钥 (OpenSSH 格式，扔给 Oracle 机房用)
	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubKeyBytes := ssh.MarshalAuthorizedKey(publicRsaKey)
	publicKey := string(pubKeyBytes)

	return privatePEM, publicKey, nil
}

// CreateVNCConnection 向 Oracle 申请开启底层串口通道
func CreateVNCConnection(accountID int, instanceID string) (core.InstanceConsoleConnection, string, error) {
	// 复用我们之前写好的底层 OCI 客户端（自带代理和解密）
	computeClient, _, _, err := buildClient(accountID)
	if err != nil {
		return core.InstanceConsoleConnection{}, "", err
	}

	// 1. 制造一次性钥匙
	privateKey, publicKey, err := GenerateSSHKeypair()
	if err != nil {
		return core.InstanceConsoleConnection{}, "", fmt.Errorf("生成临时密钥失败: %v", err)
	}

	// 2. 拿着公钥，向甲骨文申请开通这台机器的 Console 权限
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

	// 成功！返回甲骨文给的通道信息，以及我们手里的私钥
	return resp.InstanceConsoleConnection, privateKey, nil
}

// DeleteVNCConnection 扫尾工作：用完的通道必须删掉，保持云端干净
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
