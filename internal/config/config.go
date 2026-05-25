package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Name          string         `yaml:"name"`
	Host          string         `yaml:"host"`
	Port          int            `yaml:"port"`
	MySQL         *MySQL         `yaml:"mysql"`
	AdminToken    string         `yaml:"admin_token"`
	CorsOrigins   []string       `yaml:"cors_origins"`
	RateLimitRPS  int            `yaml:"rate_limit_rps"`
	BuiltinAgents []BuiltinAgent `yaml:"builtin_agents"`
	BridgeAgents  []BridgeAgent  `yaml:"bridge_agents"`
}

type BridgeAgent struct {
	Name        string        `yaml:"name" json:"name"`
	Description string        `yaml:"description" json:"description,omitempty"`
	Version     string        `yaml:"version" json:"version,omitempty"`
	Target      BridgeTarget  `yaml:"target" json:"target"`
	Skills      []BridgeSkill `yaml:"skills" json:"skills"`
}

type BridgeTarget struct {
	HTTP *BridgeHTTPTarget `yaml:"http" json:"http,omitempty"`
	CLI  *BridgeCLITarget  `yaml:"cli" json:"cli,omitempty"`
}

type BridgeHTTPTarget struct {
	BaseURL string            `yaml:"baseUrl" json:"base_url"`
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`
}

type BridgeCLITarget struct {
	Shell      string `yaml:"shell" json:"shell,omitempty"`
	WorkingDir string `yaml:"workingDir" json:"working_dir,omitempty"`
	Timeout    int    `yaml:"timeout" json:"timeout,omitempty"`
}

type BridgeSkill struct {
	ID          string      `yaml:"id" json:"id"`
	Name        string      `yaml:"name" json:"name"`
	Description string      `yaml:"description" json:"description,omitempty"`
	Tags        []string    `yaml:"tags" json:"tags,omitempty"`
	Examples    []string    `yaml:"examples" json:"examples,omitempty"`
	Invoke      SkillInvoke `yaml:"invoke" json:"invoke"`
}

type SkillInvoke struct {
	Type     string            `yaml:"type" json:"type"`
	Method   string            `yaml:"method" json:"method,omitempty"`
	Path     string            `yaml:"path" json:"path,omitempty"`
	URL      string            `yaml:"url" json:"url,omitempty"`
	Headers  map[string]string `yaml:"headers" json:"headers,omitempty"`
	Body     interface{}       `yaml:"body" json:"body,omitempty"`
	Response *ResponseExtract  `yaml:"response" json:"response,omitempty"`
	Command  string            `yaml:"command" json:"command,omitempty"`
	Args     []string          `yaml:"args" json:"args,omitempty"`
	Timeout  int               `yaml:"timeout" json:"timeout,omitempty"`
}

type ResponseExtract struct {
	Text string `yaml:"text" json:"text,omitempty"`
	Raw  bool   `yaml:"raw" json:"raw,omitempty"`
}

type MySQL struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	MaxIdle  int    `yaml:"max_idle"`
	MaxOpen  int    `yaml:"max_open"`
}

type BuiltinAgent struct {
	Name              string `yaml:"name" json:"name"`
	Provider          string `yaml:"provider" json:"provider"`
	BaseURL           string `yaml:"base_url" json:"base_url"`
	APIKey            string `yaml:"api_key" json:"-"`
	Model             string `yaml:"model" json:"model"`
	Description       string `yaml:"description" json:"description"`
	SystemPrompt      string `yaml:"system_prompt" json:"system_prompt"`
	MaxTokens         int    `yaml:"max_tokens" json:"max_tokens"`
	MaxToolRounds     int    `yaml:"max_tool_rounds" json:"max_tool_rounds"`
	MaxTurns          int    `yaml:"max_turns" json:"max_turns"`
	MaxToolResultSize int    `yaml:"max_tool_result_size" json:"max_tool_result_size"`
}

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

func expandEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		key := envVarRe.FindStringSubmatch(match)[1]
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return match
	})
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Expand environment variables before parsing
	expanded := expandEnv(string(data))

	var c Config
	if err := yaml.Unmarshal([]byte(expanded), &c); err != nil {
		return nil, err
	}
	if c.Port == 0 {
		c.Port = 18090
	}
	if c.MySQL == nil || c.MySQL.Host == "" {
		return nil, fmt.Errorf("mysql configuration is required")
	}
	if c.MySQL.Port == 0 {
		c.MySQL.Port = 3306
	}
	if c.MySQL.MaxIdle == 0 {
		c.MySQL.MaxIdle = 10
	}
	if c.MySQL.MaxOpen == 0 {
		c.MySQL.MaxOpen = 100
	}
	if len(c.CorsOrigins) == 0 {
		c.CorsOrigins = []string{"*"}
	}
	if c.RateLimitRPS == 0 {
		c.RateLimitRPS = 100
	}
	for i := range c.BuiltinAgents {
		if c.BuiltinAgents[i].MaxTokens == 0 {
			c.BuiltinAgents[i].MaxTokens = 4096
		}
		if c.BuiltinAgents[i].MaxToolRounds == 0 {
			c.BuiltinAgents[i].MaxToolRounds = 10
		}
		if c.BuiltinAgents[i].MaxTurns == 0 {
			c.BuiltinAgents[i].MaxTurns = 20
		}
		if c.BuiltinAgents[i].MaxToolResultSize == 0 {
			c.BuiltinAgents[i].MaxToolResultSize = 10000
		}
	}
	if v := os.Getenv("ADMIN_TOKEN"); v != "" {
		c.AdminToken = v
	}
	return &c, nil
}

func (m MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database)
}
