package service

import (
	"encoding/json"
)

type OCICredentials struct {
	Tenancy     string `json:"tenancy"`
	User        string `json:"user"`
	Region      string `json:"region"`
	Fingerprint string `json:"fingerprint"`
	PrivateKey  string `json:"private_key"`
}

type Account struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ProxyURL string `json:"proxy_url"`
}

func AddAccount(name, proxyURL string, creds OCICredentials) error {
	credsJSON, err := json.Marshal(creds)
	if err != nil { return err }

	encryptedCreds, err := Encrypt(string(credsJSON))
	if err != nil { return err }

	_, err = DB.Exec("INSERT INTO oci_accounts (account_name, encrypted_config, proxy_url) VALUES (?, ?, ?)",
		name, encryptedCreds, proxyURL)
	return err
}

func ListAccounts() ([]Account, error) {
	rows, err := DB.Query("SELECT id, account_name, proxy_url FROM oci_accounts")
	if err != nil { return nil, err }
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var acc Account
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.ProxyURL); err != nil { return nil, err }
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

// 🚀 新增：从数据库中彻底抹除账号记录
func DeleteAccount(id int) error {
	_, err := DB.Exec("DELETE FROM oci_accounts WHERE id = ?", id)
	return err
}
