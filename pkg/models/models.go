package models

type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

type Host struct {
	ID           string `yaml:"id" json:"id"`
	Use          string `yaml:"use" json:"use"`           
	Address      string `yaml:"address" json:"address"`
	CheckCommand string `yaml:"check_command" json:"check_command"`
	Alias        string `yaml:"alias" json:"alias"`
	Register     *bool  `yaml:"register" json:"register"` 
}

type Service struct {
	ID             string `yaml:"id" json:"id"`
	Use            string `yaml:"use" json:"use"`
	HostName       string `yaml:"host_name" json:"host_name"`
	CheckCommand   string `yaml:"check_command" json:"check_command"`
	NormalInterval int    `yaml:"normal_interval" json:"normal_interval"`
	RetryInterval  int    `yaml:"retry_interval" json:"retry_interval"`
	MaxAttempts    int    `yaml:"max_attempts" json:"max_attempts"`
	Register       *bool  `yaml:"register" json:"register"`
}

type GlobalConfig struct {
	Commands []Command `yaml:"commands" json:"commands"`
	Hosts    []Host    `yaml:"hosts" json:"hosts"`
	Services []Service `yaml:"services" json:"services"`
}
