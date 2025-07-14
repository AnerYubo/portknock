package nftmanager

import (
    "fmt"
    "net"
    "strings"
    "sync"

    "github.com/google/nftables"
    "github.com/google/nftables/expr"
    "golang.org/x/sys/unix"
	"portknock/utils"
)

// RuleKey 表示规则的唯一标识：IP + Port
type RuleKey struct {
    IP   string
    Port int
}

// Manager 负责与 nftables 交互，管理敲门规则
type Manager struct {
    conn       *nftables.Conn
    table      *nftables.Table
    blockChain *nftables.Chain // 主链 pkinput
    mutex      sync.Mutex
    rulesByIP  map[RuleKey]*nftables.Rule // 每个 (ip, port) 对应一条规则
    blockedPorts map[int]bool              // 防止重复添加 drop 规则
}

// NewManager 创建一个新的 nftables 管理器
func NewManager() *Manager {
    m := &Manager{
        conn:        &nftables.Conn{},
        table:       nil,
        blockChain:  nil,
        rulesByIP:   make(map[RuleKey]*nftables.Rule),
        blockedPorts: make(map[int]bool),
    }
    // 检查是否已有 portknock 表，有则删除
    tables, err := m.conn.ListTables()
    if err != nil {
        panic(err) // 或者处理错误
    }    
    for _, t := range tables {
        if t.Name == "portknock" && t.Family == nftables.TableFamilyINet {
            m.conn.DelTable(t)
        }
    }
    m.conn.Flush() // 先提交删除操作

    // 创建表 portknock
    m.table = m.conn.AddTable(&nftables.Table{
        Family: nftables.TableFamilyINet,
        Name:   "portknock",
    })

    // 删除旧链（可选）
    m.deleteChainIfExist("pkinput") // 主链

    // 设置主链 pkinput，默认 accept（允许所有未匹配规则的流量通过）
    defaultPolicy := nftables.ChainPolicyAccept
    blockChain := m.conn.AddChain(&nftables.Chain{
        Name:     "pkinput",
        Table:    m.table,
        Type:     nftables.ChainTypeFilter,
        Hooknum:  nftables.ChainHookInput,
        Priority: nftables.ChainPriorityFilter,
        Policy:   &defaultPolicy,
    })

    // 提交规则
    if err := m.conn.Flush(); err != nil {
        panic(err)
    }

    // 初始化字段
    m.blockChain = blockChain
    utils.LogInfo("初始化表完成")
    return m
}

// deleteChainIfExist 删除指定名称的链（如果存在）
func (m *Manager) deleteChainIfExist(chainName string) error {
    chains, err := m.conn.ListChains()
    if err != nil {
        utils.LogError("列出链失败: %v", err)
        return fmt.Errorf("列出链失败: %v", err)
    }

    for _, chain := range chains {
        if chain.Name == chainName {
            m.conn.DelChain(chain)
            utils.LogInfo("成功删除链: %s\n", chainName)
            return nil
        }
    }

    return nil
}

// AddBlockRule 阻止所有 IP 访问指定端口（TCP 和 UDP），加在主链 pkinput 上
func (m *Manager) AddBlockRule(serviceName string, port int) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    // 防止重复添加 drop 规则
    if m.blockedPorts[port] {
        return nil
    }

    // TCP 规则：阻止访问目标端口
    tcpRule := &nftables.Rule{
        Table: m.table,
        Chain: m.blockChain,
        Exprs: []expr.Any{
            // 协议 == TCP
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 9, Len: 1},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{unix.IPPROTO_TCP}},

            // 目标端口 == port
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{byte(port >> 8), byte(port & 0xff)}},

            // 拒绝
            &expr.Verdict{Kind: expr.VerdictDrop},
        },
    }

    // UDP 规则：阻止访问目标端口
    udpRule := &nftables.Rule{
        Table: m.table,
        Chain: m.blockChain,
        Exprs: []expr.Any{
            // 协议 == UDP
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 9, Len: 1},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{unix.IPPROTO_UDP}},

            // 目标端口 == port
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{byte(port >> 8), byte(port & 0xff)}},

            // 拒绝
            &expr.Verdict{Kind: expr.VerdictDrop},
        },
    }

    m.conn.AddRule(tcpRule)
    m.conn.AddRule(udpRule)
    m.blockedPorts[port] = true

    return m.conn.Flush()
}

// AllowIP 放行指定 IP 访问指定端口（只加到 allowChain 中）
func (m *Manager) AllowIP(serviceName, srcIP string, port int, expireSeconds int, allowChain *nftables.Chain) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    ip := net.ParseIP(srcIP).To4()
    if ip == nil {
        utils.LogError("invalid ipv4")
        return fmt.Errorf("invalid ipv4")
    }
    
    key := RuleKey{IP: srcIP, Port: port}
    // 获取当前链上所有规则
    rules, GetRulesErr := m.conn.GetRules(m.table, allowChain)
    if GetRulesErr != nil {
        utils.LogError("[nft] 获取规则失败: %v\n", GetRulesErr)
        return GetRulesErr
    }

    found := false
    for _, rule := range rules {
        expected := fmt.Sprintf("service:%s,ip:%s,port:%d", serviceName, ip, port)
        if strings.HasPrefix(string(rule.UserData), expected) {
            found = true
        }
    }

    if found {
        utils.LogWarn("[nft] 规则存在不添加\n")
        return nil
    }
    // 删除旧规则（防止重复添加）
    for k, r := range m.rulesByIP {
        if k == key {
            m.conn.DelRule(r)
            delete(m.rulesByIP, k)
        }
    }
    
    rule := &nftables.Rule{
        Table: m.table,
        Chain: allowChain, // 使用传入的专属链
        Exprs: []expr.Any{
            // 匹配源 IP 地址
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 12, Len: 4},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: ip},

            // 匹配目标端口
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{byte(port >> 8), byte(port & 0xff)}},

            // 放行
            &expr.Verdict{Kind: expr.VerdictAccept},
        },
        UserData: []byte(fmt.Sprintf("service:%s,ip:%s,port:%d", serviceName, srcIP, port)),
    }

    m.conn.AddRule(rule)
    err := m.conn.Flush()
    if err != nil {
        utils.LogError("[%s] 添加规则失败: %v\n", serviceName, err)
        return err
    }

    m.rulesByIP[key] = rule
    utils.LogInfo("[nft] 成功添加规则: %s -> %d (handle=%d)\n", srcIP, port, rule.Handle)
    return nil
}

// RevokeIP 撤销指定 IP 的放行规则
func (m *Manager) RevokeIP(serviceName, ip string, port int, allowChain *nftables.Chain) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    key := RuleKey{IP: ip, Port: port}
    delete(m.rulesByIP, key)

    // 获取当前链上所有规则
    rules, err := m.conn.GetRules(m.table, allowChain)
    if err != nil {
        utils.LogError("[nft] 获取规则失败: %v\n", err)
        return err
    }

    found := false
    for _, rule := range rules {
        expected := fmt.Sprintf("service:%s,ip:%s,port:%d", serviceName, ip, port)
        if strings.HasPrefix(string(rule.UserData), expected) {
            utils.LogInfo("[nft] 找到规则并准备删除: %s (handle=%d)\n", expected, rule.Handle)
            m.conn.DelRule(rule)
            found = true
        }
    }

    if !found {
        utils.LogWarn("[nft] 未找到规则: %s:%d\n", ip, port)
        return nil
    }

    utils.LogInfo("[nft] 已提交删除请求: %s:%d\n", ip, port)
    return m.conn.Flush()
}

// CreateAllowChain 为某个服务创建专属放行链，并插入跳转规则到主链
func (m *Manager) CreateAllowChain(serviceName string, port int) (*nftables.Chain, error) {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    chainName := fmt.Sprintf("%s_allow", serviceName)

    // 如果已存在该链，先删除
    chains, err := m.conn.ListChains()
    if err != nil {
        return nil, err
    }
    for _, c := range chains {
        if c.Name == chainName && c.Table == m.table {
            m.conn.DelChain(c)
        }
    }
    m.conn.Flush()

    // 创建新的专属链
    allowChain := m.conn.AddChain(&nftables.Chain{
        Name:  chainName,
        Table: m.table,
        Type:  nftables.ChainTypeFilter,
    })

    // 插入 jump 到该链的规则（主链 pkinput）
    jumpRule := &nftables.Rule{
        Exprs: []expr.Any{
            // TCP 协议 + 目标端口匹配
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 9, Len: 1},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{unix.IPPROTO_TCP}},
            &expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
            &expr.Cmp{Register: 1, Op: expr.CmpOpEq, Data: []byte{byte(port >> 8), byte(port & 0xff)}},
            &expr.Verdict{Kind: expr.VerdictJump, Chain: chainName}, // 跳转到专属链
        },
        UserData: []byte(fmt.Sprintf("jump-%s", chainName)),
    }
    jumpRule.Table = m.table
    jumpRule.Chain = m.blockChain
    m.conn.InsertRule(jumpRule)

    // 提交规则
    err = m.conn.Flush()
    if err != nil {
        return nil, err
    }
    utils.LogInfo("为 %d 端口创建 %s 表成功",port, serviceName)
    return allowChain, nil
}


// Conn 导出 conn 字段
func (m *Manager) Conn() *nftables.Conn {
    return m.conn
}

// Table 导出 table 字段
func (m *Manager) Table() *nftables.Table {
    return m.table
}

// MainChain 导出主链
func (m *Manager) MainChain() *nftables.Chain {
    return m.blockChain
}