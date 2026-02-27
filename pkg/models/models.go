package models

// TimePeriod defines a weekly schedule for checks or notifications (Nagios-style)
type TimePeriod struct {
	ID        string   `yaml:"id" json:"id"`
	Alias     string   `yaml:"alias" json:"alias"`
	Monday    []string `yaml:"monday" json:"monday"`
	Tuesday   []string `yaml:"tuesday" json:"tuesday"`
	Wednesday []string `yaml:"wednesday" json:"wednesday"`
	Thursday  []string `yaml:"thursday" json:"thursday"`
	Friday    []string `yaml:"friday" json:"friday"`
	Saturday  []string `yaml:"saturday" json:"saturday"`
	Sunday    []string `yaml:"sunday" json:"sunday"`
}

// Contact defines a person or group to receive alerts
type Contact struct {
	ID    string `yaml:"id" json:"id"`
	Email string `yaml:"email" json:"email"`
}

// HostGroup and ServiceGroup allow logical clustering of objects
type HostGroup struct {
	ID    string `yaml:"id" json:"id"`
	Alias string `yaml:"alias" json:"alias"`
}

type ServiceGroup struct {
	ID    string `yaml:"id" json:"id"`
	Alias string `yaml:"alias" json:"alias"`
}

// Command represents the executable line with potential macros
type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

// Host represents a monitored machine with template inheritance capabilities
type Host struct {
	ID                 string            `yaml:"id" json:"id"`
	Use                string            `yaml:"use" json:"use"` // Template ID to inherit from
	Address            string            `yaml:"address" json:"address"`
	CheckCommand       string            `yaml:"check_command" json:"check_command"`
	CheckPeriod        string            `yaml:"check_period" json:"check_period"`
	NotificationPeriod string            `yaml:"notification_period" json:"notification_period"`
	Alias              string            `yaml:"alias" json:"alias"`
	Contacts           []string          `yaml:"contacts" json:"contacts"`
	HostGroups         []string          `yaml:"hostgroups" json:"hostgroups"`
	Macros             map[string]string `yaml:"macros" json:"macros"`
	Register           *bool             `yaml:"register" json:"register"` // If false, object is a template
}

// Service represents a check associated with a specific Host
type Service struct {
	ID                 string            `yaml:"id" json:"id"`
	Use                string            `yaml:"use" json:"use"`
	HostName           string            `yaml:"host_name" json:"host_name"`
	CheckCommand       string            `yaml:"check_command" json:"check_command"`
	CheckPeriod        string            `yaml:"check_period" json:"check_period"`
	NotificationPeriod string            `yaml:"notification_period" json:"notification_period"`
	NormalInterval     int               `yaml:"normal_interval" json:"normal_interval"`
	RetryInterval      int               `yaml:"retry_interval" json:"retry_interval"`
	MaxAttempts        int               `yaml:"max_attempts" json:"max_attempts"`
	Contacts           []string          `yaml:"contacts" json:"contacts"`
	ServiceGroups      []string          `yaml:"servicegroups" json:"servicegroups"`
	Macros             map[string]string `yaml:"macros" json:"macros"`
	Register           *bool             `yaml:"register" json:"register"`
}

// GlobalConfig is the final consolidated structure sent to the Scheduler
type GlobalConfig struct {
	Commands      []Command      `yaml:"commands" json:"commands"`
	Contacts      []Contact      `yaml:"contacts" json:"contacts"`
	TimePeriods   []TimePeriod   `yaml:"timeperiods" json:"timeperiods"`
	HostGroups    []HostGroup    `yaml:"hostgroups" json:"hostgroups"`
	ServiceGroups []ServiceGroup `yaml:"servicegroups" json:"servicegroups"`
	Hosts         []Host         `yaml:"hosts" json:"hosts"`
	Services      []Service      `yaml:"services" json:"services"`
}
