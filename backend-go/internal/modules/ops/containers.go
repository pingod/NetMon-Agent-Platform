package ops

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// dockerCommandClient 抽象 SSH 远端与本机 socket 两种 Docker 执行方式。
type dockerCommandClient interface {
	Close() error
	ExecCommand(cmd string, timeout time.Duration) (string, string, error)
	ExecCommandInput(cmd string, input string, timeout time.Duration) (string, string, error)
}

type dockerSSHClient struct {
	client *ssh.Client
}

func (c *dockerSSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *dockerSSHClient) ExecCommand(cmd string, timeout time.Duration) (string, string, error) {
	if c.client == nil {
		return "", "", fmt.Errorf("ssh client is nil")
	}
	sess, err := c.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer sess.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	sess.Stdout = &stdoutBuf
	sess.Stderr = &stderrBuf

	done := make(chan error, 1)
	go func() {
		done <- sess.Run(cmd)
	}()

	select {
	case <-time.After(timeout):
		_ = sess.Signal(ssh.SIGKILL)
		// 关闭会话让 Run 返回，并等 goroutine 结束后再读缓冲区，避免数据竞争
		_ = sess.Close()
		<-done
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timed out after %v", timeout)
	case err := <-done:
		return stdoutBuf.String(), stderrBuf.String(), err
	}
}

// ExecCommandInput 与 ExecCommand 相同，但把 input 写入远端命令的 stdin。
func (c *dockerSSHClient) ExecCommandInput(cmd string, input string, timeout time.Duration) (string, string, error) {
	if c.client == nil {
		return "", "", fmt.Errorf("ssh client is nil")
	}
	sess, err := c.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer sess.Close()
	var stdoutBuf, stderrBuf bytes.Buffer
	sess.Stdout = &stdoutBuf
	sess.Stderr = &stderrBuf
	sess.Stdin = strings.NewReader(input)
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case <-time.After(timeout):
		_ = sess.Signal(ssh.SIGKILL)
		_ = sess.Close()
		<-done
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command timed out after %v", timeout)
	case err := <-done:
		return stdoutBuf.String(), stderrBuf.String(), err
	}
}

// containerRefPattern 容器 ID / 名称 / 镜像标签白名单：首字符字母数字，其余仅允许字母数字与 _.:-
var containerRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)

func validContainerRef(s string) bool {
	return s != "" && len(s) <= 256 && containerRefPattern.MatchString(s)
}

// shellQuote 单引号转义，确保参数不会被目标 shell 解释
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// portMappingPattern 端口映射白名单：host:container[/proto] 或 container[/proto]
var portMappingPattern = regexp.MustCompile(`^[0-9A-Za-z.:\[\]-]+(/[a-zA-Z]+)?$`)

func validPortMapping(s string) bool {
	return portMappingPattern.MatchString(s)
}

// envPair 允许 KEY=VALUE，KEY 受标识符约束，VALUE 不允许换行
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validEnvPair(s string) bool {
	if strings.ContainsAny(s, "\r\n\x00") {
		return false
	}
	idx := strings.Index(s, "=")
	if idx <= 0 {
		return false
	}
	return envNamePattern.MatchString(s[:idx])
}

// validVolumeSpec 宿主路径:容器路径[:ro|rw]，拒绝换行与命令替换字符
func validVolumeSpec(s string) bool {
	if s == "" || strings.ContainsAny(s, "\r\n\x00;&|`$()") {
		return false
	}
	return strings.Contains(s, ":")
}

var restartPolicyPattern = regexp.MustCompile(`^(no|always|unless-stopped|on-failure(:\d+)?)$`)

// respondHostUnavailable 宿主机不可用时返回 502，绝不回退到伪造数据
func respondHostUnavailable(w http.ResponseWriter, err error) {
	httpx.RespondFailStatus(w, http.StatusBadGateway, "无法连接 Docker 宿主机: "+err.Error())
}

// parseDockerBytes 解析 docker 命令输出的容量字符串（如 5.727MiB / 414B / 1.5GB）
func parseDockerBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" {
		return 0, s == "0B"
	}
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	unit := strings.TrimSpace(s[i:])
	mult := map[string]float64{
		"B": 1, "kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12,
		"KiB": 1024, "MiB": 1024 * 1024, "GiB": 1024 * 1024 * 1024, "TiB": 1024 * 1024 * 1024 * 1024,
	}
	m, ok := mult[unit]
	if !ok {
		return 0, false
	}
	return int64(num * m), true
}

// splitDockerPair 拆分 docker 输出中的 "a / b" 容量对
func splitDockerPair(s string) (int64, int64, bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	used, ok1 := parseDockerBytes(parts[0])
	limit, ok2 := parseDockerBytes(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return used, limit, true
}

// getDockerSSHClient 获取目标 Docker 宿主机连接。
// 未指定/未纳管任何服务器时，若容器内挂载了 /var/run/docker.sock，则退化到本机 Docker。
func getDockerSSHClient(endpointID string) (dockerCommandClient, string, error) {
	db := appdb.DB()
	if db == nil {
		return nil, "", fmt.Errorf("数据库未初始化")
	}
	// 未显式指定远端端点时，优先使用本机 socket（容器化部署的默认形态）。
	if (endpointID == "" || endpointID == "local") && ensureLocalDocker() == nil {
		return &localDockerClient{}, "local", nil
	}

	var serverID, host string
	var port int
	var username, password, privateKey string
	var useSSHKey int

	// 1. 如果指定了 endpointID 且非 local，优先匹配 servers 表或 container_endpoints 表
	if endpointID != "" && endpointID != "local" {
		err := db.QueryRow(`SELECT id, hostname, port FROM servers WHERE id = ? OR hostname = ? OR ip_address = ?`, endpointID, endpointID, endpointID).Scan(&serverID, &host, &port)
		if err == nil {
			username, password, privateKey, useSSHKey, _ = repos.Servers.GetCredentialFields(serverID)
		} else {
			var epHost string
			var epPort int
			if db.QueryRow(`SELECT host, port FROM container_endpoints WHERE id = ?`, endpointID).Scan(&epHost, &epPort) == nil {
				if db.QueryRow(`SELECT id, hostname, port FROM servers WHERE hostname = ? OR ip_address = ?`, epHost, epHost).Scan(&serverID, &host, &port) == nil {
					username, password, privateKey, useSSHKey, _ = repos.Servers.GetCredentialFields(serverID)
				}
			}
		}
	}

	// 2. 如果未指定或 local，优先寻找运行了 agent 或纳管的 Linux 服务器（如 180.76.184.2）
	if host == "" {
		rows, err := db.Query(`SELECT id, hostname, port FROM servers WHERE enabled = 1 ORDER BY agent_installed DESC, is_virtual_machine DESC, created_at ASC LIMIT 1`)
		if err == nil {
			defer rows.Close()
			if rows.Next() {
				_ = rows.Scan(&serverID, &host, &port)
				username, password, privateKey, useSSHKey, _ = repos.Servers.GetCredentialFields(serverID)
			}
		}
	}

	if host == "" {
		if err := ensureLocalDocker(); err == nil {
			return &localDockerClient{}, "local", nil
		} else if endpointID == "local" {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("未找到可用的 Docker 宿主机（可在 compose 中挂载 /var/run/docker.sock）")
	}
	if port <= 0 {
		port = 22
	}
	if username == "" {
		username = "root"
	}

	var authMethods []ssh.AuthMethod
	if useSSHKey == 1 && privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
		authMethods = append(authMethods, ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			ans := make([]string, len(questions))
			for i := range questions {
				ans[i] = password
			}
			return ans, nil
		}))
	}

	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		// 优先尝试本机 socket：容器化部署通常没有可 SSH 的宿主机。
		if localErr := ensureLocalDocker(); localErr == nil {
			return &localDockerClient{}, "local", nil
		}
		return nil, host, fmt.Errorf("连接宿主机 SSH 失败 (%s:%d): %v", host, port, err)
	}
	return &dockerSSHClient{client: conn}, host, nil
}

// DockerMonitorRoutes 容器监控专属路由 (/api/v1/docker-monitor)
func DockerMonitorRoutes() chi.Router {
	r := chi.NewRouter()

	// 1. 集群快照聚合指标
	r.Get("/cluster-snapshot", func(w http.ResponseWriter, r *http.Request) {
		endpointID := r.URL.Query().Get("endpointId")
		client, _, err := getDockerSSHClient(endpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()

		stdout, _, err := client.ExecCommand("docker stats --no-stream --format '{{json .}}'", 8*time.Second)
		if err != nil {
			respondHostUnavailable(w, fmt.Errorf("采集容器指标失败: %w", err))
			return
		}
		lines := strings.Split(strings.TrimSpace(stdout), "\n")

		total := 0
		running := 0
		var totalMemUsage, totalMemLimit int64
		var totalCPU float64

		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			total++
			running++
			var m map[string]interface{}
			if json.Unmarshal([]byte(l), &m) == nil {
				cpuStr := strings.TrimSuffix(fmt.Sprintf("%v", m["CPUPerc"]), "%")
				if v, err := strconv.ParseFloat(cpuStr, 64); err == nil {
					totalCPU += v
				}
				if used, limit, ok := splitDockerPair(fmt.Sprintf("%v", m["MemUsage"])); ok {
					totalMemUsage += used
					totalMemLimit += limit
				}
			}
		}

		memPct := 0.0
		if totalMemLimit > 0 {
			memPct = float64(totalMemUsage) / float64(totalMemLimit) * 100
		}
		httpx.Respond(w, map[string]interface{}{
			"totalContainers":    total,
			"runningContainers":  running,
			"totalCpuPercent":    fmt.Sprintf("%.1f%%", totalCPU),
			"totalMemoryUsage":   totalMemUsage,
			"totalMemoryLimit":   totalMemLimit,
			"totalMemoryPercent": fmt.Sprintf("%.2f%%", memPct),
		})
	})

	return r
}

var initEndpointsOnce sync.Once

func initContainerEndpointsTable() {
	initEndpointsOnce.Do(func() {
		db := appdb.DB()
		if db == nil {
			return
		}
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS container_endpoints (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 2375,
			protocol TEXT NOT NULL DEFAULT 'tcp',
			status TEXT NOT NULL DEFAULT 'active',
			tls_ca TEXT,
			tls_cert TEXT,
			tls_key TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)

		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS deleted_endpoints (
			id TEXT PRIMARY KEY,
			deleted_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
	})
}

// DockerHostsRoute 获取全部可用 Docker 主机列表
func DockerHostsRoute(w http.ResponseWriter, r *http.Request) {
	initContainerEndpointsTable()
	db := appdb.DB()
	var list []map[string]interface{}
	if db != nil {
		rows, err := db.Query(`SELECT id, name, hostname, port FROM servers WHERE enabled = 1 AND id NOT IN (SELECT id FROM deleted_endpoints) ORDER BY agent_installed DESC, created_at ASC`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, name, host string
				var port int
				if rows.Scan(&id, &name, &host, &port) == nil {
					if port <= 0 {
						port = 22
					}
					list = append(list, map[string]interface{}{
						"id":       id,
						"name":     fmt.Sprintf("%s (远程宿主机)", name),
						"host":     host,
						"port":     port,
						"protocol": "ssh",
						"status":   "active",
					})
				}
			}
		}

		// 自定义端点（排除已删除的）
		epRows, err := db.Query(`SELECT id, name, host, port, protocol, status FROM container_endpoints WHERE id NOT IN (SELECT id FROM deleted_endpoints) ORDER BY created_at ASC`)
		if err == nil {
			defer epRows.Close()
			for epRows.Next() {
				var id, name, host, protocol, status string
				var port int
				if epRows.Scan(&id, &name, &host, &port, &protocol, &status) == nil {
					list = append(list, map[string]interface{}{
						"id":       id,
						"name":     name,
						"host":     host,
						"port":     port,
						"protocol": protocol,
						"status":   status,
					})
				}
			}
		}
	}
	if ensureLocalDocker() == nil {
		list = append(list, map[string]interface{}{
			"id": "local", "name": "本机 Docker", "host": "127.0.0.1", "port": 0,
			"protocol": "local", "status": "active",
		})
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	httpx.Respond(w, list)
}

// FullContainerRoutes 全功能容器管理与监控接口路由
func FullContainerRoutes() chi.Router {
	r := chi.NewRouter()

	// ── Docker 主机与端点 ──
	r.Get("/hosts", DockerHostsRoute)
	r.Get("/endpoints", func(w http.ResponseWriter, r *http.Request) {
		initContainerEndpointsTable()
		db := appdb.DB()
		var list []map[string]interface{}
		if db != nil {
			rows, err := db.Query(`SELECT id, name, hostname, port FROM servers WHERE enabled = 1 AND id NOT IN (SELECT id FROM deleted_endpoints) ORDER BY agent_installed DESC, created_at ASC`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var id, name, host string
					var port int
					if rows.Scan(&id, &name, &host, &port) == nil {
						if port <= 0 {
							port = 22
						}
						list = append(list, map[string]interface{}{
							"id":       id,
							"name":     fmt.Sprintf("%s (Linux VM)", name),
							"host":     host,
							"port":     port,
							"protocol": "ssh",
							"status":   "active",
						})
					}
				}
			}

			// 查询用户添加的自定义端点（排除已删除的；TLS 私钥不出 HTTP 层，仅返回是否配置）
			epRows, err := db.Query(`SELECT id, name, host, port, protocol, status, tls_ca, tls_cert, tls_key FROM container_endpoints WHERE id NOT IN (SELECT id FROM deleted_endpoints) ORDER BY created_at ASC`)
			if err == nil {
				defer epRows.Close()
				for epRows.Next() {
					var id, name, host, protocol, status string
					var port int
					var tlsCa, tlsCert, tlsKey sql.NullString
					if epRows.Scan(&id, &name, &host, &port, &protocol, &status, &tlsCa, &tlsCert, &tlsKey) == nil {
						list = append(list, map[string]interface{}{
							"id":       id,
							"name":     name,
							"host":     host,
							"port":     port,
							"protocol": protocol,
							"status":   status,
							"hasTls":   tlsCa.String != "" || tlsCert.String != "" || tlsKey.String != "",
						})
					}
				}
			}
		}
		if ensureLocalDocker() == nil {
			list = append(list, map[string]interface{}{
				"id": "local", "name": "本机 Docker", "host": "127.0.0.1", "port": 0,
				"protocol": "local", "status": "active",
			})
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin")).Post("/endpoints/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Port <= 0 {
			req.Port = 2375
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(req.Host, strconv.Itoa(req.Port)), 3*time.Second)
		if err != nil {
			httpx.Respond(w, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("TCP 端口 %d 无法连通: %v", req.Port, err),
			})
			return
		}
		conn.Close()
		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("端点 %s:%d 连接测试成功", req.Host, req.Port),
		})
	})

	r.With(middleware.RequireRole("admin")).Post("/endpoints", func(w http.ResponseWriter, r *http.Request) {
		initContainerEndpointsTable()
		var req struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
			TlsCa    string `json:"tlsCa"`
			TlsCert  string `json:"tlsCert"`
			TlsKey   string `json:"tlsKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		if req.Host == "" {
			httpx.RespondError(w, "主机地址不能为空", http.StatusBadRequest)
			return
		}
		if req.Port <= 0 {
			req.Port = 2375
		}
		if req.Name == "" {
			if req.Port == 6379 {
				req.Name = fmt.Sprintf("%s (Redis)", req.Host)
			} else {
				req.Name = fmt.Sprintf("%s:%d", req.Host, req.Port)
			}
		}
		if req.Protocol == "" {
			req.Protocol = "tcp"
		}

		epID := fmt.Sprintf("ep-%d", time.Now().UnixNano())

		status := "active"
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(req.Host, strconv.Itoa(req.Port)), 2*time.Second)
		if err == nil {
			conn.Close()
			status = "active"
		}

		db := appdb.DB()
		if db != nil {
			_, _ = db.Exec(`DELETE FROM deleted_endpoints WHERE id = ?`, epID)
			_, err = db.Exec(`INSERT INTO container_endpoints (id, name, host, port, protocol, status, tls_ca, tls_cert, tls_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
				epID, req.Name, req.Host, req.Port, req.Protocol, status, req.TlsCa, req.TlsCert, req.TlsKey)
			if err != nil {
				logger.Warnf("保存容器端点失败: %v", err)
			}
		}

		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "端点添加成功",
			"data": map[string]interface{}{
				"id":       epID,
				"name":     req.Name,
				"host":     req.Host,
				"port":     req.Port,
				"protocol": req.Protocol,
				"status":   status,
			},
		})
	})

	r.With(middleware.RequireRole("admin")).Put("/endpoints/{id}", func(w http.ResponseWriter, r *http.Request) {
		initContainerEndpointsTable()
		id := chi.URLParam(r, "id")
		var req struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
			TlsCa    string `json:"tlsCa"`
			TlsCert  string `json:"tlsCert"`
			TlsKey   string `json:"tlsKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		db := appdb.DB()
		if db != nil {
			// TLS 字段留空表示不修改（列表接口不再下发密钥内容，编辑表单无法回填）
			if req.TlsCa == "" || req.TlsCert == "" || req.TlsKey == "" {
				var oldCa, oldCert, oldKey sql.NullString
				_ = db.QueryRow(`SELECT tls_ca, tls_cert, tls_key FROM container_endpoints WHERE id = ?`, id).Scan(&oldCa, &oldCert, &oldKey)
				if req.TlsCa == "" {
					req.TlsCa = oldCa.String
				}
				if req.TlsCert == "" {
					req.TlsCert = oldCert.String
				}
				if req.TlsKey == "" {
					req.TlsKey = oldKey.String
				}
			}
			_, err := db.Exec(`UPDATE container_endpoints SET name = ?, host = ?, port = ?, protocol = ?, tls_ca = ?, tls_cert = ?, tls_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				req.Name, req.Host, req.Port, req.Protocol, req.TlsCa, req.TlsCert, req.TlsKey, id)
			if err != nil {
				httpx.RespondError(w, "更新端点失败: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "端点已更新",
		})
	})

	r.With(middleware.RequireRole("admin")).Delete("/endpoints/{id}", func(w http.ResponseWriter, r *http.Request) {
		initContainerEndpointsTable()
		id := chi.URLParam(r, "id")
		db := appdb.DB()
		if db != nil {
			_, _ = db.Exec(`DELETE FROM container_endpoints WHERE id = ?`, id)
			_, _ = db.Exec(`INSERT OR REPLACE INTO deleted_endpoints (id) VALUES (?)`, id)
		}
		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "端点已删除",
		})
	})

	r.With(middleware.RequireRole("admin")).Post("/endpoints/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		initContainerEndpointsTable()
		id := chi.URLParam(r, "id")
		db := appdb.DB()
		var host string
		var port int
		if db != nil {
			_ = db.QueryRow(`SELECT host, port FROM container_endpoints WHERE id = ?`, id).Scan(&host, &port)
			if host != "" {
				status := "active"
				conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 2*time.Second)
				if err != nil {
					status = "inactive"
				} else {
					conn.Close()
				}
				_, _ = db.Exec(`UPDATE container_endpoints SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
			}
		}
		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "刷新成功",
		})
	})

	// ── 容器列表（含实时规格与指标） ──
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		endpointID := r.URL.Query().Get("endpointId")
		client, host, err := getDockerSSHClient(endpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()

		var containersList []map[string]interface{}

		{

			// 1. 获取容器实时列表
			stdout, _, _ := client.ExecCommand("docker ps -a --format '{{json .}}'", 8*time.Second)

			// 2. 获取实时容器 stats
			statsMap := map[string]map[string]interface{}{}
			statsOut, _, _ := client.ExecCommand("docker stats --no-stream --format '{{json .}}'", 8*time.Second)
			for _, sl := range strings.Split(strings.TrimSpace(statsOut), "\n") {
				sl = strings.TrimSpace(sl)
				if sl == "" {
					continue
				}
				var sm map[string]interface{}
				if json.Unmarshal([]byte(sl), &sm) == nil {
					cID := fmt.Sprintf("%v", sm["ID"])
					cName := fmt.Sprintf("%v", sm["Name"])
					cpuStr := fmt.Sprintf("%v", sm["CPUPerc"])
					memUsageStr := fmt.Sprintf("%v", sm["MemUsage"])
					memPctStr := fmt.Sprintf("%v", sm["MemPerc"])
					netIOStr := fmt.Sprintf("%v", sm["NetIO"])
					pidsStr := fmt.Sprintf("%v", sm["PIDs"])
					rxBytes, _ := parseDockerBytes(strings.TrimSpace(strings.SplitN(netIOStr, "/", 2)[0]))
					var txBytes int64
					if netParts := strings.SplitN(netIOStr, "/", 2); len(netParts) == 2 {
						txBytes, _ = parseDockerBytes(netParts[1])
					}

					memBytes, limitBytes, _ := splitDockerPair(memUsageStr)

					pids, _ := strconv.Atoi(pidsStr)

					statObj := map[string]interface{}{
						"containerId": cID,
						"cpuPercent":  cpuStr,
						"memory": map[string]interface{}{
							"usage":   memBytes,
							"limit":   limitBytes,
							"percent": strings.TrimSuffix(memPctStr, "%"),
						},
						"network": map[string]interface{}{
							"rx_bytes": rxBytes,
							"tx_bytes": txBytes,
						},
						"pids": pids,
					}
					statsMap[cID] = statObj
					statsMap[cName] = statObj
				}
			}

			// 3. 解析容器行
			for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var ps map[string]interface{}
				if json.Unmarshal([]byte(line), &ps) == nil {
					id := fmt.Sprintf("%v", ps["ID"])
					name := fmt.Sprintf("%v", ps["Names"])
					image := fmt.Sprintf("%v", ps["Image"])
					state := strings.ToLower(fmt.Sprintf("%v", ps["State"]))
					status := fmt.Sprintf("%v", ps["Status"])
					rawPorts := fmt.Sprintf("%v", ps["Ports"])
					createdStr := fmt.Sprintf("%v", ps["CreatedAt"])

					var portsList []map[string]interface{}
					rePort := regexp.MustCompile(`(?:[0-9.]+|\[::\]):(\d+)->(\d+)/(\w+)`)
					matches := rePort.FindAllStringSubmatch(rawPorts, -1)
					for _, m := range matches {
						pub, _ := strconv.Atoi(m[1])
						priv, _ := strconv.Atoi(m[2])
						portsList = append(portsList, map[string]interface{}{
							"IP":          "0.0.0.0",
							"PublicPort":  pub,
							"PrivatePort": priv,
							"Type":        m[3],
						})
					}
					if len(portsList) == 0 && strings.Contains(rawPorts, "/") {
						reSingle := regexp.MustCompile(`(\d+)/(\w+)`)
						for _, sm := range reSingle.FindAllStringSubmatch(rawPorts, -1) {
							priv, _ := strconv.Atoi(sm[1])
							portsList = append(portsList, map[string]interface{}{
								"PrivatePort": priv,
								"Type":        sm[2],
							})
						}
					}

					var cStat map[string]interface{}
					if s, ok := statsMap[id]; ok {
						cStat = s
					} else if s, ok := statsMap[name]; ok {
						cStat = s
					}

					// docker ps 的 CreatedAt 形如 "2026-11-10 22:14:03 +0800 CST"，解析为 Unix 秒
					var createdUnix int64
					if t, perr := time.Parse("2006-01-02 15:04:05 -0700 MST", createdStr); perr == nil {
						createdUnix = t.Unix()
					}

					containersList = append(containersList, map[string]interface{}{
						"id":           id,
						"container_id": id,
						"name":         name,
						"Names":        []string{"/" + name},
						"image":        image,
						"Image":        image,
						"status":       state,
						"state":        state,
						"State":        state,
						"Status":       status,
						"ports":        portsList,
						"Ports":        portsList,
						"host":         host,
						"created_at":   createdStr,
						"created":      createdUnix,
						"Created":      createdUnix,
						"stats":        cStat,
					})
				}
			}
		}

		if containersList == nil {
			containersList = []map[string]interface{}{}
		}

		// 服务端搜索 / 状态过滤 / 分页（前端一直传这些参数，此前被忽略）
		search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
		statusFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
		if search != "" || statusFilter != "" {
			filtered := make([]map[string]interface{}, 0, len(containersList))
			for _, c := range containersList {
				if statusFilter != "" && fmt.Sprintf("%v", c["state"]) != statusFilter {
					continue
				}
				if search != "" {
					haystack := strings.ToLower(fmt.Sprintf("%v %v %v", c["name"], c["image"], c["id"]))
					if !strings.Contains(haystack, search) {
						continue
					}
				}
				filtered = append(filtered, c)
			}
			containersList = filtered
		}

		total := len(containersList)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if page > 0 && pageSize > 0 {
			if pageSize > 200 {
				pageSize = 200
			}
			start := (page - 1) * pageSize
			if start > total {
				start = total
			}
			end := start + pageSize
			if end > total {
				end = total
			}
			containersList = containersList[start:end]
		}

		httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    containersList,
			"total":   total,
		})
	})

	// ── 容器详情 ──
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的容器 ID")
			return
		}
		endpointID := r.URL.Query().Get("endpointId")
		client, _, err := getDockerSSHClient(endpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		stdout, stderr, execErr := client.ExecCommand("docker inspect "+shellQuote(id), 8*time.Second)
		if execErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("查询容器详情失败: %v %s", execErr, stderr))
			return
		}
		var inspectArr []map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &inspectArr); err != nil || len(inspectArr) == 0 {
			httpx.RespondFailStatus(w, http.StatusNotFound, "容器不存在")
			return
		}
		httpx.Respond(w, inspectArr[0])
	})

	// ── 容器日志 ──
	r.Get("/logs/{id}", handleGetContainerLogs)
	r.Get("/{id}/logs", handleGetContainerLogs)

	// ── 容器实时 Stats ──
	r.Get("/stats/{id}", handleGetContainerStats)
	r.Get("/{id}/stats", handleGetContainerStats)

	// ── 容器操作 (start/stop/restart) ──
	r.With(middleware.RequireRole("admin")).Post("/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		action := chi.URLParam(r, "action")
		endpointID := r.URL.Query().Get("endpointId")

		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的容器 ID")
			return
		}
		if action != "start" && action != "stop" && action != "restart" {
			httpx.RespondError(w, "不支持的操作类型", http.StatusBadRequest)
			return
		}

		client, host, err := getDockerSSHClient(endpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()

		stdout, stderr, cmdErr := client.ExecCommand("docker "+action+" "+shellQuote(id), 15*time.Second)
		if cmdErr != nil {
			logger.Warnf("执行 docker %s 失败 (%s): %v, stderr: %s", action, host, cmdErr, stderr)
			httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("执行 docker %s 失败: %v %s", action, cmdErr, stderr))
			return
		}

		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("容器 %s 已成功执行 %s", id, action),
			"output":  stdout,
		})
	})

	// ── 创建并运行容器 ──
	r.With(middleware.RequireRole("admin")).Post("/run", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Image         string   `json:"image"`
			Name          string   `json:"name"`
			Ports         []string `json:"ports"`
			Env           []string `json:"env"`
			Volumes       []string `json:"volumes"`
			RestartPolicy string   `json:"restartPolicy"`
			Memory        int64    `json:"memory"`
			CPUShares     int      `json:"cpuShares"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析错误", http.StatusBadRequest)
			return
		}
		if !validContainerRef(req.Image) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的镜像名称")
			return
		}
		if req.Name != "" && !validContainerRef(req.Name) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的容器名称")
			return
		}
		for _, p := range req.Ports {
			if !validPortMapping(p) {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的端口映射: "+p)
				return
			}
		}
		for _, e := range req.Env {
			if !validEnvPair(e) {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的环境变量: "+e)
				return
			}
		}
		for _, v := range req.Volumes {
			if !validVolumeSpec(v) {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的数据卷映射: "+v)
				return
			}
		}
		restartPolicy := req.RestartPolicy
		if restartPolicy == "" {
			restartPolicy = "no"
		}
		if !restartPolicyPattern.MatchString(restartPolicy) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的重启策略")
			return
		}

		endpointID := r.URL.Query().Get("endpointId")
		client, host, err := getDockerSSHClient(endpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()

		args := []string{"docker", "run", "-d"}
		if req.Name != "" {
			args = append(args, "--name", req.Name)
		}
		for _, p := range req.Ports {
			args = append(args, "-p", p)
		}
		for _, e := range req.Env {
			args = append(args, "-e", e)
		}
		for _, v := range req.Volumes {
			args = append(args, "-v", v)
		}
		if restartPolicy != "no" {
			args = append(args, "--restart", restartPolicy)
		}
		if req.Memory > 0 {
			args = append(args, "-m", strconv.FormatInt(req.Memory, 10))
		}
		args = append(args, req.Image)

		quoted := make([]string, len(args))
		for i, a := range args {
			quoted[i] = shellQuote(a)
		}

		stdout, stderr, cmdErr := client.ExecCommand(strings.Join(quoted, " "), 60*time.Second)
		if cmdErr != nil {
			logger.Warnf("创建容器失败 (%s): %v, stderr: %s", host, cmdErr, stderr)
			httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("创建容器失败: %v %s", cmdErr, stderr))
			return
		}

		httpx.Respond(w, map[string]interface{}{
			"success": true,
			"message": "容器创建成功",
			"id":      strings.TrimSpace(stdout),
		})
	})

	// ── 删除容器 ──
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的容器 ID")
			return
		}
		endpointID := r.URL.Query().Get("endpointId")
		client, _, err := getDockerSSHClient(endpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()

		_, stderr, cmdErr := client.ExecCommand("docker rm -f "+shellQuote(id), 15*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "删除容器失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "容器已删除"})
	})

	// ── 镜像列表 ──
	r.Get("/images", handleListDockerImages)
	r.Get("/images/list", handleListDockerImages)

	// ── 数据卷列表 ──
	r.Get("/volumes", handleListDockerVolumes)
	r.Get("/volumes/list", handleListDockerVolumes)

	// ── 网络列表 ──
	r.Get("/networks", handleListDockerNetworks)
	r.Get("/networks/list", handleListDockerNetworks)

	// ── 拉取镜像 ──
	r.With(middleware.RequireRole("admin")).Post("/images/pull", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Image string `json:"image"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		if !validContainerRef(req.Image) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的镜像名称")
			return
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		stdout, stderr, cmdErr := client.ExecCommand("docker pull "+shellQuote(req.Image), 120*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("拉取镜像失败: %v %s", cmdErr, stderr))
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "镜像拉取成功", "output": stdout})
	})

	// ── 删除镜像 ──
	r.With(middleware.RequireRole("admin")).Delete("/images/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的镜像 ID")
			return
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand("docker rmi "+shellQuote(id), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "删除镜像失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "镜像已删除"})
	})

	// ── 创建数据卷 ──
	r.With(middleware.RequireRole("admin")).Post("/volumes", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name   string `json:"name"`
			Driver string `json:"driver"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		if !validContainerRef(req.Name) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的数据卷名称")
			return
		}
		cmd := "docker volume create "
		if req.Driver != "" && req.Driver != "local" {
			if !validContainerRef(req.Driver) {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的数据卷驱动")
				return
			}
			cmd += "--driver " + shellQuote(req.Driver) + " "
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand(cmd+shellQuote(req.Name), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "创建数据卷失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "数据卷创建成功", "name": req.Name})
	})

	// ── 删除数据卷 ──
	r.With(middleware.RequireRole("admin")).Delete("/volumes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的数据卷名称")
			return
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand("docker volume rm "+shellQuote(id), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "删除数据卷失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "数据卷已删除"})
	})

	// ── 创建网络 ──
	r.With(middleware.RequireRole("admin")).Post("/networks", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string `json:"name"`
			Driver     string `json:"driver"`
			Subnet     string `json:"subnet"`
			Gateway    string `json:"gateway"`
			Internal   bool   `json:"internal"`
			Attachable bool   `json:"attachable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		if !validContainerRef(req.Name) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的网络名称")
			return
		}
		cmd := "docker network create "
		if req.Driver != "" {
			if !validContainerRef(req.Driver) {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的网络驱动")
				return
			}
			cmd += "--driver " + shellQuote(req.Driver) + " "
		}
		if req.Subnet != "" {
			if _, _, err := net.ParseCIDR(req.Subnet); err != nil {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的子网 CIDR")
				return
			}
			cmd += "--subnet " + shellQuote(req.Subnet) + " "
		}
		if req.Gateway != "" {
			if net.ParseIP(req.Gateway) == nil {
				httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的网关地址")
				return
			}
			cmd += "--gateway " + shellQuote(req.Gateway) + " "
		}
		if req.Internal {
			cmd += "--internal "
		}
		if req.Attachable {
			cmd += "--attachable "
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		stdout, stderr, cmdErr := client.ExecCommand(cmd+shellQuote(req.Name), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "创建网络失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "网络已创建", "id": strings.TrimSpace(stdout)})
	})

	// ── 删除网络 ──
	r.With(middleware.RequireRole("admin")).Delete("/networks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的网络 ID")
			return
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand("docker network rm "+shellQuote(id), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "删除网络失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "网络已删除"})
	})

	// ── Compose 项目 ──
	r.Get("/compose", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []interface{}{})
	})

	return r
}

// handleSyncDockerImages 重新盘点指定 Docker 主机的镜像清单（列表本身即实时数据，
// sync 的语义是连通性校验 + 返回当前镜像数）。
func handleSyncDockerImages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServerID   string `json:"serverId"`
		EndpointID string `json:"endpointId"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&req)
	}
	id := req.ServerID
	if id == "" {
		id = req.EndpointID
	}
	client, host, err := getDockerSSHClient(id)
	if err != nil {
		respondHostUnavailable(w, err)
		return
	}
	defer client.Close()
	stdout, stderr, execErr := client.ExecCommand("docker images --format '{{json .}}'", 15*time.Second)
	if execErr != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("同步镜像失败: %v %s", execErr, stderr))
		return
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	httpx.Respond(w, map[string]interface{}{"synced": count, "host": host})
}

func handleListDockerImages(w http.ResponseWriter, r *http.Request) {
	endpointID := r.URL.Query().Get("endpointId")
	client, host, err := getDockerSSHClient(endpointID)
	if err != nil {
		respondHostUnavailable(w, err)
		return
	}
	defer client.Close()
	stdout, stderr, execErr := client.ExecCommand("docker images --format '{{json .}}'", 8*time.Second)
	if execErr != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("获取镜像列表失败: %v %s", execErr, stderr))
		return
	}
	var list []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(line), &m) == nil {
			repo := fmt.Sprintf("%v", m["Repository"])
			tag := fmt.Sprintf("%v", m["Tag"])
			sizeStr := fmt.Sprintf("%v", m["Size"])
			id := fmt.Sprintf("%v", m["ID"])
			created := fmt.Sprintf("%v", m["CreatedSince"])

			sizeBytes, _ := parseDockerBytes(sizeStr)

			list = append(list, map[string]interface{}{
				"id":         id,
				"name":       repo,
				"tag":        tag,
				"size_bytes": sizeBytes,
				"size":       sizeBytes,
				"host":       host,
				"created_at": created,
			})
		}
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    list,
		"total":   len(list),
	})
}

func handleListDockerVolumes(w http.ResponseWriter, r *http.Request) {
	endpointID := r.URL.Query().Get("endpointId")
	client, host, err := getDockerSSHClient(endpointID)
	if err != nil {
		respondHostUnavailable(w, err)
		return
	}
	defer client.Close()
	stdout, stderr, execErr := client.ExecCommand("docker volume ls --format '{{json .}}'", 8*time.Second)
	if execErr != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("获取数据卷列表失败: %v %s", execErr, stderr))
		return
	}
	var list []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(line), &m) == nil {
			name := fmt.Sprintf("%v", m["Name"])
			driver := fmt.Sprintf("%v", m["Driver"])
			list = append(list, map[string]interface{}{
				"id":          name,
				"name":        name,
				"driver":      driver,
				"type":        "docker",
				"host":        host,
				"mount_point": fmt.Sprintf("/var/lib/docker/volumes/%s/_data", name),
				"status":      "available",
			})
		}
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    list,
		"total":   len(list),
	})
}

func handleListDockerNetworks(w http.ResponseWriter, r *http.Request) {
	endpointID := r.URL.Query().Get("endpointId")
	client, _, err := getDockerSSHClient(endpointID)
	if err != nil {
		respondHostUnavailable(w, err)
		return
	}
	defer client.Close()
	stdout, stderr, execErr := client.ExecCommand("docker network ls --format '{{json .}}'", 8*time.Second)
	if execErr != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("获取网络列表失败: %v %s", execErr, stderr))
		return
	}
	var list []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(line), &m) == nil {
			id := fmt.Sprintf("%v", m["ID"])
			name := fmt.Sprintf("%v", m["Name"])
			driver := fmt.Sprintf("%v", m["Driver"])
			scope := fmt.Sprintf("%v", m["Scope"])
			list = append(list, map[string]interface{}{
				"id":     id,
				"Id":     id,
				"name":   name,
				"Name":   name,
				"driver": driver,
				"Driver": driver,
				"scope":  scope,
				"Scope":  scope,
			})
		}
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    list,
		"total":   len(list),
	})
}

func handleGetContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validContainerRef(id) {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的容器 ID")
		return
	}
	tail := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("tail")); err == nil && v >= 1 && v <= 5000 {
		tail = v
	}
	cmd := "docker logs --tail " + strconv.Itoa(tail)
	if r.URL.Query().Get("timestamps") == "1" {
		cmd += " --timestamps"
	}
	endpointID := r.URL.Query().Get("endpointId")
	client, _, err := getDockerSSHClient(endpointID)
	if err != nil {
		respondHostUnavailable(w, err)
		return
	}
	defer client.Close()
	stdout, stderr, execErr := client.ExecCommand(cmd+" "+shellQuote(id), 8*time.Second)
	if execErr != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("获取容器日志失败: %v %s", execErr, stderr))
		return
	}
	// docker 把容器 stdout/stderr 分别写到本地两个流，合并返回避免丢行
	logs := stdout + stderr
	httpx.Respond(w, logs)
}

func handleGetContainerStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validContainerRef(id) {
		httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的容器 ID")
		return
	}
	endpointID := r.URL.Query().Get("endpointId")
	client, _, err := getDockerSSHClient(endpointID)
	if err != nil {
		respondHostUnavailable(w, err)
		return
	}
	defer client.Close()
	stdout, stderr, execErr := client.ExecCommand("docker stats --no-stream --format '{{json .}}' "+shellQuote(id), 8*time.Second)
	if execErr != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("获取容器指标失败: %v %s", execErr, stderr))
		return
	}
	var sm map[string]interface{}
	if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &sm) != nil {
		httpx.RespondFailStatus(w, http.StatusBadGateway, "无法解析容器指标输出")
		return
	}
	httpx.Respond(w, sm)
}

// ImagesRoutes 镜像管理独立路由 (/api/v1/images)
func ImagesRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", handleListDockerImages)
	r.With(middleware.RequireRole("admin")).Post("/sync", handleSyncDockerImages)
	r.With(middleware.RequireRole("admin")).Post("/pull", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Image      string `json:"image"`
			EndpointID string `json:"endpointId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		if !validContainerRef(req.Image) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的镜像名称")
			return
		}
		client, _, err := getDockerSSHClient(req.EndpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		stdout, stderr, cmdErr := client.ExecCommand("docker pull "+shellQuote(req.Image), 120*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, fmt.Sprintf("拉取镜像失败: %v %s", cmdErr, stderr))
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "镜像拉取成功", "output": stdout})
	})
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的镜像 ID")
			return
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand("docker rmi "+shellQuote(id), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "删除镜像失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "镜像已删除"})
	})
	return r
}

// VolumesRoutes 数据卷管理独立路由 (/api/v1/volumes)
func VolumesRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", handleListDockerVolumes)
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string `json:"name"`
			EndpointID string `json:"endpointId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "参数解析失败", http.StatusBadRequest)
			return
		}
		if !validContainerRef(req.Name) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的数据卷名称")
			return
		}
		client, _, err := getDockerSSHClient(req.EndpointID)
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand("docker volume create "+shellQuote(req.Name), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "创建数据卷失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "数据卷创建成功", "name": req.Name})
	})
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !validContainerRef(id) {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "非法的数据卷名称")
			return
		}
		client, _, err := getDockerSSHClient(r.URL.Query().Get("endpointId"))
		if err != nil {
			respondHostUnavailable(w, err)
			return
		}
		defer client.Close()
		_, stderr, cmdErr := client.ExecCommand("docker volume rm "+shellQuote(id), 30*time.Second)
		if cmdErr != nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "删除数据卷失败: "+stderr)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "数据卷已删除"})
	})
	return r
}
