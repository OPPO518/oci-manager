package service

import (
	"encoding/json"
)

// OCICredentials 定义了连接 Oracle 所需的核心要素
type OCICredentials struct {
	Tenancy     string `json:"tenancy"`
	User        string `json:"user"`
	Region      string `json:"region"`
	Fingerprint string `json:"fingerprint"`
	PrivateKey  string `json:"private_key"`
}

// Account 对应数据库里查出来的账号信息
type Account struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ProxyURL string `json:"proxy_url"`
	// 注意：为了绝对安全，我们绝不会把私钥发送给网页前端
}

// AddAccount 添加新账号，并将秘钥加密入库
func AddAccount(name, proxyURL string, creds OCICredentials) error {
	// 1. 把包含私钥的信息打包成 JSON 字符串
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	// 2. 调用安保队长，把 JSON 加密成乱码
	encryptedCreds, err := Encrypt(string(credsJSON))
	if err != nil {
		return err
	}

	// 3. 存入数据库
	_, err = DB.Exec("INSERT INTO oci_accounts (account_name, encrypted_config, proxy_url) VALUES (?, ?, ?)",
		name, encryptedCreds, proxyURL)
	return err
}

// ListAccounts 获取保险箱里的所有账号列表
func ListAccounts() ([]Account, error) {
	rows, err := DB.Query("SELECT id, account_name, proxy_url FROM oci_accounts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var acc Account
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.ProxyURL); err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}
