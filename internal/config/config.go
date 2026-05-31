package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config 是应用的顶层配置结构体,包含所有子模块的配置
// 每个字段对应 config.yaml 中的一个顶级节点
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`   // HTTP 服务配置
	Database DatabaseConfig `mapstructure:"database"` // PostgreSQL 连接配置
	DeepSeek DeepSeekConfig `mapstructure:"deepseek"` // DeepSeek APT 配置
	RAG      RAGConfig      `mapstructure:"rag"`      // RAG 管道参数配置
}

// ServerConfig HTTP 服务器的连接和超时配置
type ServerConfig struct {
	Port         int `mapstructure:"port"`          // 监听端口,默认 8080
	ReadTimeout  int `mapstructure:"read_timeout"`  // 读取请求超时,如 30s
	WriteTimeout int `mapstructure:"write_timeout"` // 写入响应超时, SEE 需要较长时间
}

// DatabaseConfig PostgreSQL 数据库连接参数
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`     // 数据库主机地址
	Port     int    `mapstructure:"port"`     // 数据库端口,默认 5432
	Name     string `mapstructure:"name"`     // 数据库名称
	User     string `mapstructure:"user"`     // 数据库用户名
	Password string `mapstructure:"password"` // 密码,通过环境变量注入,不写在 YAML 中
	SSLMode  string `mapstructure:"ssl_mode"` // SSL 模式
}

// DSN 返回 PostgreSQL 连接字符串 (Data Source Name)
// 格式：postgres://user:password@host:port/dbname?sslmode=mode
// pgx 驱动使用 URL 格式的连接串
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

// DeepSeekConfig DeepSeek API (OpenAI 兼容接口) 的配置
type DeepSeekConfig struct {
	BaseURL        string  `mapstructure:"base_url"`        // APT 地址,如 https://api.deepseek.com/v4
	ChatModel      string  `mapstructure:"chat_model"`      // 对话模型名称, 如 DeepSeek-V4-pro
	EmbeddingModel string  `mapstructure:"embedding_model"` // Embedding 模型名称
	MaxTokens      int     `mapstructure:"max_tokens"`      // 单词回答最大 token 数
	Temperature    float64 `mapstructure:"temperature"`     // 生成温度(0 ~ 1),知识问答建议 0.3 以下
}

// RAGConfig RAG(检索增强生成)管道的参数配置
// 这些参数直接影响检索质量和回答准确性，可能需要根据实际效果调优
type RAGConfig struct {
	ChunkSize           int     `mapstructure:"chunk_size"`           // 每个文本切片的 token 数上限
	ChunkOverlap        int     `mapstructure:"chunk_overlap"`        // 相邻切片之间的重叠 token shu
	TopK                int     `mapstructure:"top_k"`                // 检索返回的最相关片段数量
	SimilarityThreshold float64 `mapstructure:"similarity_threshold"` // 余弦相似度最低阈值(0~1)
	MaxHistoryRounds    int     `mapstructure:"max_history_rounds"`   // 对话历史保留轮数
	SystemPrompt        string  `mapstructure:"system_prompt"`        // System Prompt 模板
}

// Load 加载配置文件, configPath 为空时使用默认路径
//
// 加载顺序 (后面的覆盖前面的)
// 1. 代码中的默认值: setDefault
// 2. config.yaml 文件中的值
// 3. 环境变量 (AutomaticEnv + BindEnv)
//
// 这样设计的好处：开发时用 YAML 方便修改，部署时敏感信息通过环境变量注入
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// 如果指定了配置文件路径,直接使用
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 否则在当前目录查找 config.yaml
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.SetConfigType("yaml")
	}

	// --- 设置默认值 ---
	// 这些值在 config.yaml 不存在对应字段时生效
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "120s")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("deepseek.max_tokens", 4096)
	v.SetDefault("deepseek.temperature", 0.3)
	v.SetDefault("rag.chunk_size", 500)           // 中文约 400 字
	v.SetDefault("rag.chunk_overlap", 50)         // 10% 重叠率
	v.SetDefault("rag.top_k", 5)                  // 返回最相关的 5 个片段
	v.SetDefault("rag.similarity_threshold", 0.7) // 相似度低于 0.7 的片段不采用
	v.SetDefault("rag_max_history_rounds", 5)     // 保留最近 5 轮对话
	v.SetDefault("rag_system_prompt", "你是一个企业知识库问答助手."+
		"<规则>"+
		"\n1. 仅根据以下参考资料的內容回答问题\n"+
		"2. 如果参考资料中沒有相关信息，请明确告知用户\"当前知识库中未找到相关信息\"\n"+
		"3. 回答时引用具体的文档名称\n"+
		"4. 保持回答简洁、专业、结构化\n"+
		"</规则>`")
	// 启动环境变量自动映射
	// 例如：database.password 映射到环境变量 DB_PASSWORD
	v.AutomaticEnv()
	err := v.BindEnv("database.password", "DB_PASSWORD")
	if err != nil {
		return nil, fmt.Errorf("读取数据库的环境变量发送错误, %w", err)
	}
	err = v.BindEnv("deepseek.api_key", "DEEPSEEK_API_KEY")
	if err != nil {
		return nil, fmt.Errorf("读取DeepSeek API key 发送错误, %w", err)
	}

	// 尝试读取配置文件
	// 文件不存在时不报错(允许纯环境变量运行),其他错误才返回
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置文件失败, %w", err)
		}
	}

	// 将 Viper 中的数据反序列化到 Config 结构体
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败, %w", err)
	}

	// 密码和 API Key 从环境变量获取
	cfg.Database.Password = os.Getenv("DB_PASSWORD")
	if cfg.Database.Password == "" {
		cfg.Database.Password = v.GetString("database.password")
	}

	return &cfg, nil
}
