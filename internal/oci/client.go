// Package oci 封装 Oracle Cloud Infrastructure 的 API 调用。
// 核心设计：每个账号、每个区域持有一组独立的客户端，且客户端绑定该账号专属的
// HTTP 代理，从而实现出口 IP 隔离，避免多账号共用同一 IP 触发风控。
package oci

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/usageapi"

	"github.com/yourname/oci-manager/internal/store"
)

// Credentials 是调用 OCI API 所需的完整凭证。
type Credentials struct {
	TenancyOCID   string
	UserOCID      string
	Fingerprint   string
	PrivateKeyPEM string
	Passphrase    string
}

// ClientSet 是某个账号在某个区域下的一组服务客户端。
type ClientSet struct {
	Region     string
	Compute    *core.ComputeClient
	Identity   *identity.IdentityClient
	Usage      *usageapi.UsageapiClient
	Network    *core.VirtualNetworkClient
}

// newClientSet 构建一组客户端。proxyURL 为空表示直连。
func newClientSet(creds Credentials, region, proxyURL string) (*ClientSet, error) {
	var passphrase *string
	if creds.Passphrase != "" {
		p := creds.Passphrase
		passphrase = &p
	}

	// 凭证完全来自内存，不读取 ~/.oci/config，因此天然支持多账号并存。
	provider := common.NewRawConfigurationProvider(
		creds.TenancyOCID,
		creds.UserOCID,
		region,
		creds.Fingerprint,
		creds.PrivateKeyPEM,
		passphrase,
	)

	httpClient, err := newHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	compute, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建 Compute 客户端失败: %w", err)
	}
	compute.HTTPClient = httpClient

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建 Identity 客户端失败: %w", err)
	}
	identityClient.HTTPClient = httpClient

	usageClient, err := usageapi.NewUsageapiClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建 Usage 客户端失败: %w", err)
	}
	usageClient.HTTPClient = httpClient

	networkClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建网络客户端失败: %w", err)
	}
	networkClient.HTTPClient = httpClient

	return &ClientSet{
		Region:   region,
		Compute:  &compute,
		Identity: &identityClient,
		Usage:    &usageClient,
		Network:  &networkClient,
	}, nil
}

// newHTTPClient 构造绑定指定代理的 http.Client。
// 支持 http、https 与 socks5 三种代理协议，形如：
//
//	http://user:pass@1.2.3.4:8080
//	socks5://user:pass@1.2.3.4:1080
func newHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("代理地址格式错误: %w", err)
		}
		switch parsed.Scheme {
		case "http", "https", "socks5":
		default:
			return nil, fmt.Errorf("不支持的代理协议 %q，仅支持 http/https/socks5", parsed.Scheme)
		}
		transport.Proxy = http.ProxyURL(parsed)
	}

	return &http.Client{Transport: transport, Timeout: 90 * time.Second}, nil
}

// Pool 缓存账号客户端，避免每次请求都重新解析私钥与建立连接。
// 凭证或代理发生变化时会自动重建客户端。
type Pool struct {
	mu    sync.Mutex
	store *store.Store
	cache map[string]*poolEntry
}

type poolEntry struct {
	clients     *ClientSet
	fingerprint string
}

// NewPool 创建客户端池。
func NewPool(st *store.Store) *Pool {
	return &Pool{store: st, cache: make(map[string]*poolEntry)}
}

// For 返回指定账号在指定区域下的客户端。region 为空时使用账号的默认区域。
func (p *Pool) For(accountID, region string) (*ClientSet, error) {
	account, err := p.store.GetAccount(accountID)
	if err != nil {
		return nil, err
	}
	secret, err := p.store.AccountSecret(accountID)
	if err != nil {
		return nil, err
	}
	if region == "" {
		region = account.Region
	}

	creds := Credentials{
		TenancyOCID:   account.TenancyOCID,
		UserOCID:      account.UserOCID,
		Fingerprint:   account.Fingerprint,
		PrivateKeyPEM: secret.PrivateKeyPEM,
		Passphrase:    secret.Passphrase,
	}
	fingerprint := credentialsFingerprint(creds, region, account.ProxyURL)

	key := accountID + "|" + region
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.cache[key]; ok && entry.fingerprint == fingerprint {
		return entry.clients, nil
	}

	clients, err := newClientSet(creds, region, account.ProxyURL)
	if err != nil {
		return nil, err
	}
	p.cache[key] = &poolEntry{clients: clients, fingerprint: fingerprint}
	return clients, nil
}

// Invalidate 丢弃某个账号的全部缓存客户端，修改账号信息后调用。
func (p *Pool) Invalidate(accountID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.cache {
		if len(key) >= len(accountID) && key[:len(accountID)] == accountID {
			delete(p.cache, key)
		}
	}
}

// credentialsFingerprint 计算凭证摘要，用于判断客户端是否需要重建。
// 摘要不含私钥明文以外的敏感信息泄露风险，仅用于内存比较。
func credentialsFingerprint(creds Credentials, region, proxyURL string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s", creds.TenancyOCID, creds.UserOCID,
		creds.Fingerprint, creds.PrivateKeyPEM, region, proxyURL)
	return hex.EncodeToString(h.Sum(nil))
}
