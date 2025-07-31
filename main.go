package main

import (
    "log"
    "sync"
    "time"

    "github.com/google/gopacket"
    "github.com/google/gopacket/afpacket"
    "github.com/google/gopacket/layers"

    "github.com/google/nftables"

	"portknock/utils"
    "portknock/config"
    "portknock/nftmanager"
)

type KnockState struct {
    SeqIndex     int
    LastTime     time.Time
    AllowedUntil time.Time
}

type KnockServer struct {
    cfg           *config.ServiceConfig
    nft           *nftmanager.Manager
    stateMap      map[string]*KnockState
    mu            sync.Mutex
    portToService map[uint16]string
    allowChain    *nftables.Chain // 每个服务有自己独立的 allowChain
}

func NewKnockServer(cfg *config.ServiceConfig, nft *nftmanager.Manager, portToService map[uint16]string) *KnockServer {
    // ✅ 使用 Manager 创建专属 allowChain
    allowChain, err := nft.CreateAllowChain(cfg.Name, int(cfg.AllowPort))
    if err != nil {
        log.Fatalf("[%s] 创建专属链失败: %v", cfg.Name, err)
    }

    // ✅ 初始化服务结构
    server := &KnockServer{
        cfg:           cfg,
        nft:           nft,
        stateMap:      make(map[string]*KnockState),
        portToService: portToService,
        allowChain:    allowChain,
    }

    // ✅ 添加白名单 IP（一次性写入 rules）
    for _, ip := range cfg.Whitelist {
        err := nft.AllowIP(cfg.Name, ip, int(cfg.AllowPort), cfg.ExpireSeconds, allowChain)
        if err != nil {
            utils.LogError("[%s] 添加白名单 %s 失败: %v", cfg.Name, ip, err)
        } else {
            utils.LogInfo("[%s] 已添加白名单 IP: %s", cfg.Name, ip)
            // 不加定时器，白名单永久有效（也可加一个极长的超时）
        }
    }

    return server
}

func (s *KnockServer) BlockAll() error {
    return s.nft.AddBlockRule(s.cfg.Name, int(s.cfg.AllowPort))
}

func (s *KnockServer) HandlePacket(srcIP string, dstPort int) {
    knockPorts := s.cfg.KnockPorts
    allowPort := int(s.cfg.AllowPort)

    // 判断是否是本服务关注的端口之一（KnockPorts 或 AllowPort）
    isRelevant := false
    if dstPort == allowPort {
        isRelevant = true
    } else {
        for _, p := range knockPorts {
            if dstPort == p {
                isRelevant = true
                break
            }
        }
    }

    if !isRelevant {
        return // 不属于当前服务的关注端口，直接返回
    }

    // 打印访问日志
    //serviceName := getServiceNameByPort(s.portToService, uint16(dstPort))
    // 直接使用当前服务名（无需查表）
    serviceName := s.cfg.Name    
    // 如果是 AllowPort 并且不在允许范围内，拒绝访问
    if dstPort == allowPort {
        s.mu.Lock()
        state, ok := s.stateMap[srcIP]
        s.mu.Unlock()

        if !ok || time.Now().After(state.AllowedUntil) {
            utils.LogWarn("[%s] %s 尝试直接访问放行端口 %d，拒绝访问", serviceName, srcIP, dstPort)
        }
        return
    }

    // 如果是 KnockPort，进入敲门逻辑
    for _, p := range knockPorts {
        if dstPort == p {
            utils.LogInfo("[%s] %s 访问了序列端口: %d\n", serviceName, srcIP, dstPort)
            s.handleKnock(srcIP, dstPort, time.Duration(s.cfg.ExpireSeconds)*time.Second, time.Duration(s.cfg.StepTimeoutSeconds)*time.Second)
            break
        }
    }
}

func (s *KnockServer) handleKnock(srcIP string, dstPort int, globalTimeout time.Duration, stepTimeout time.Duration) {
    s.mu.Lock()
    defer s.mu.Unlock()

    state, ok := s.stateMap[srcIP]
    now := time.Now()

    if !ok || now.Sub(state.LastTime) > globalTimeout {
        state = &KnockState{SeqIndex: 0}
    }

    expectPort := s.cfg.KnockPorts[state.SeqIndex]

    // 🚨 如果访问的不是期望端口，不管是不是放行期间，都清空状态
    if dstPort != expectPort {
        if state.SeqIndex > 0 {
            utils.LogWarn("[%s] %s 敲错端口 %d，期望 %d，已重置敲门状态\n",
                s.cfg.Name, srcIP, dstPort, expectPort)
            state.SeqIndex = 0
            state.LastTime = now
            s.stateMap[srcIP] = state
        }
        return
    }

    // ✅ 访问的是期望端口，继续流程
    state.SeqIndex++
    state.LastTime = now
    utils.LogInfo("[%s] %s 敲中了第 %d 步端口 %d\n",
        s.cfg.Name, srcIP, state.SeqIndex, dstPort)

    if state.SeqIndex == len(s.cfg.KnockPorts) {
        utils.LogInfo("[%s] %s 敲门成功，刷新放行时间\n", s.cfg.Name, srcIP)

        // 刷新允许时间
        state.AllowedUntil = now.Add(globalTimeout)

        // 更新 nftables 规则的生效时间（可选）
        err := s.nft.AllowIP(s.cfg.Name, srcIP, int(s.cfg.AllowPort), s.cfg.ExpireSeconds, s.allowChain)
        if err != nil {
            utils.LogError("[%s] 放行失败: %v\n", s.cfg.Name, err)
        } else {
            // 启动定时器删除规则
            go func(ip, serviceName string, port int) {
                <-time.After(globalTimeout)
                s.nft.RevokeIP(serviceName, ip, port, s.allowChain)
                utils.LogInfo("[%s] %s 授权过期，已撤销放行规则\n", serviceName, ip)
            }(srcIP, s.cfg.Name, int(s.cfg.AllowPort))
        }

        state.SeqIndex = 0
        s.stateMap[srcIP] = state
    } else {
        s.stateMap[srcIP] = state
    }
}

func getServiceNameByPort(portMap map[uint16]string, port uint16) string {
    if name, ok := portMap[port]; ok {
        return name
    }
    return "unknown"
}

func contains(ports []int, port int) bool {
    for _, p := range ports {
        if p == port {
            return true
        }
    }
    return false
}

func runInterfaceListener(interfaceName string, services []*config.ServiceConfig, nft *nftmanager.Manager, portToService map[uint16]string, wg *sync.WaitGroup) {
    defer wg.Done()

    handle, err := afpacket.NewTPacket(
        afpacket.OptInterface(interfaceName),
        afpacket.OptFrameSize(65536),
    )
    if err != nil {
        utils.LogWarn("创建抓包失败 (%s): %v", interfaceName, err)
        return
    }
    defer handle.Close()

    source := gopacket.NewPacketSource(handle, layers.LayerTypeEthernet)

    var servers []*KnockServer
    for _, svc := range services {
        server := NewKnockServer(svc, nft, portToService)
        servers = append(servers, server)

        err := server.BlockAll()
        if err != nil {
            utils.LogInfo("[%s] 阻断所有IP访问目标端口失败: %v", svc.Name, err)
        } else {
            utils.LogInfo("🔔  服务 %s 监听网卡 %s，敲门序列 %v，放行端口 %d\n",
                svc.Name, svc.Interface, svc.KnockPorts, svc.AllowPort)
        }
    }

    for packet := range source.Packets() {
        netL := packet.NetworkLayer()
        transL := packet.TransportLayer()
        if netL == nil || transL == nil {
            continue
        }

        ip4, ok := netL.(*layers.IPv4)
        if !ok {
            continue
        }

        var dstPort int
        var isKnock bool

        if tcp, ok := transL.(*layers.TCP); ok {
            if tcp.SYN && !tcp.ACK {
                dstPort = int(tcp.DstPort)
                isKnock = true
            }
        } else if udp, ok := transL.(*layers.UDP); ok {
            dstPort = int(udp.DstPort)
            isKnock = true
        }

        if !isKnock {
            continue
        }

        srcIP := ip4.SrcIP.String()

        for _, server := range servers {
            if dstPort == int(server.cfg.AllowPort) || contains(server.cfg.KnockPorts, dstPort) {
                go server.HandlePacket(srcIP, dstPort)
            }else{
                server.resetStateIfInvalidAccess(srcIP, dstPort)
            }
        }
    }
}
func (s *KnockServer) resetStateIfInvalidAccess(srcIP string, dstPort int) bool {
    if dstPort == int(s.cfg.AllowPort) {
        return false
    }
    for _, p := range s.cfg.KnockPorts {
        if dstPort == p {
            return false
        }
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    state, ok := s.stateMap[srcIP]
    if !ok {
        return false
    }

    now := time.Now()

    // 判断是否处于放行状态
    if state.AllowedUntil.IsZero() || now.After(state.AllowedUntil) {
        // ✅ 没有授权或授权已过期：删除整个状态
        delete(s.stateMap, srcIP)
        utils.LogError("[%s] %s 授权已过期或未获得授权，访问了无关端口 %d，已删除敲门状态\n",
            s.cfg.Name, srcIP, dstPort)
    } else {
        // ❌ 还在放行期间：只清空 SeqIndex
        state.SeqIndex = 0
        s.stateMap[srcIP] = state
        utils.LogWarn("[%s] %s 当前处于放行期间，访问了无关端口 %d，已重置 SeqIndex\n",
            s.cfg.Name, srcIP, dstPort)
    }

    return true
}
func main() {
    logFilePath := "/var/log/portknock/app.log"
    if err := utils.InitLogger(logFilePath); err != nil {
        log.Fatalf("日志初始化失败: %v", err)
    }    
    // ✅ 使用 utils 管理配置
    if err := utils.EnsureConfigFileExists(); err != nil {
        log.Fatalf("配置检查失败: %v", err)
    }

    cfg, err := utils.LoadAndValidateConfig()
    if err != nil {
        log.Fatalf("加载配置失败: %v", err)
    }

    utils.LogInfo("加载了 %d 个服务:\n", len(cfg.Services))

    portToService := make(map[uint16]string)
    for _, svc := range cfg.Services {
        portToService[svc.AllowPort] = svc.Name
        utils.LogInfo("服务名称: %s, 放行端口: %d, 敲门序列: %v, 网卡: %s",
            svc.Name, svc.AllowPort, svc.KnockPorts, svc.Interface)
    }

    nft := nftmanager.NewManager()

    interfaceMap := make(map[string][]*config.ServiceConfig)
    for i := range cfg.Services {
        svc := &cfg.Services[i]
        interfaceMap[svc.Interface] = append(interfaceMap[svc.Interface], svc)
    }

    var wg sync.WaitGroup
    for intf, svcs := range interfaceMap {
        wg.Add(1)
        go func(interfaceName string, services []*config.ServiceConfig) {
            runInterfaceListener(interfaceName, services, nft, portToService, &wg)
        }(intf, svcs)
    }

    wg.Wait()
}