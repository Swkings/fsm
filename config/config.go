// Package config 定义 FSM、持久化与可视化服务可共享的配置模型。
package config

const (
	StoreDriverJSONL = "jsonl"
	StoreDriverMySQL = "mysql"
	StoreDriverRedis = "redis"
)

// Config 是 FSM 子系统的通用配置。第三方应用可将其声明为自身配置的
// 顶层 FSM 字段，使持久化与可视化配置统一位于 FSM 命名空间下。
type Config struct {
	Store         Store         `json:"store,optional"`
	Visualization Visualization `json:"visualization,optional"`
}

// Store 配置 FSM 持久化使用的存储。Driver 为空时根据连接配置推断，
// 没有数据库配置时默认使用 JSONL。
type Store struct {
	Driver string `json:"driver,optional"`
	JSONL  JSONL  `json:"jsonl,optional"`
	MySQL  MySQL  `json:"mysql,optional"`
	Redis  Redis  `json:"redis,optional"`
}

// StoreSource 描述可被管理界面动态挂载的数据源。
type StoreSource struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Store
}

type JSONL struct {
	Directory string `json:"directory,optional"`
	Filename  string `json:"filename,optional"`
	SSH       *SSH   `json:"ssh,optional"`
}

// SSH 配置通过 SFTP 访问远程 JSONL 目录。KnownHostsFile 为空时使用
// ~/.ssh/known_hosts；仅在受信网络中才应启用 InsecureIgnoreHostKey。
type SSH struct {
	Address               string `json:"address"`
	Username              string `json:"username,optional"`
	Password              string `json:"password,optional"`
	PrivateKeyFile        string `json:"privateKeyFile,optional"`
	PrivateKeyPassphrase  string `json:"privateKeyPassphrase,optional"`
	KnownHostsFile        string `json:"knownHostsFile,optional"`
	InsecureIgnoreHostKey bool   `json:"insecureIgnoreHostKey,optional"`
	TimeoutSeconds        int    `json:"timeoutSeconds,optional"`
}

type MySQL struct {
	DSN   string `json:"dsn,optional"`
	Table string `json:"table,optional"`
}

type Redis struct {
	Address   string `json:"address,optional"`
	Password  string `json:"password,optional"`
	DB        int    `json:"db,optional"`
	KeyPrefix string `json:"keyPrefix,optional"`
}

// Visualization 配置可视化 HTTP Server。FSM 数据持久化不属于该配置；
// Replay 数据源通过 Server.RegisterReplayStore 单独挂载。
type Visualization struct {
	ServerAddress string
	TraceLimit    int
	PathPrefix    string
}

// VisualizationServer 配置非全局可视化 Server。
type VisualizationServer struct {
	Address    string
	TraceLimit int
}
