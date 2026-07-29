// Package config 定义 FSM、持久化与可视化服务可共享的配置模型。
package config

const (
	StoreDriverJSONL = "jsonl"
	StoreDriverMySQL = "mysql"
	StoreDriverRedis = "redis"
)

// Config 是可直接映射 YAML/JSON 顶层结构的通用配置，第三方应用可以嵌入
// 或直接复用，并在旁边增加自己的业务字段。
type Config struct {
	FSM           FSM           `json:"fsm,optional"`
	Store         Store         `json:"store,optional"`
	Visualization Visualization `json:"visualization,optional"`
}

// FSM 描述可由配置确定的状态机实例信息。
type FSM struct {
	Name         string `json:"name,optional"`
	InitialState string `json:"initialState,optional"`
	StoreKey     string `json:"storeKey,optional"`
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
