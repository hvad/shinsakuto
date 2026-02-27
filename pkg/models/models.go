package models

import "time"

// Downtime defines a maintenance window for a host or a service
type Downtime struct {
	ID        string    `yaml:"id" json:"id"`
	HostName  string    `yaml:"host_name" json:"host_name"`
	ServiceID string    `yaml:"service_id" json:"service_id"` // Optional: if empty, it's a host-wide downtime
	StartTime time.Time `yaml:"start_time" json:"start_time"`
	EndTime   time.Time `yaml:"end_time" json:"end_time"`
	Author    string    `yaml:"author" json:"author"`
	Comment   string    `yaml:"comment" json:"comment"`
}

// TimePeriod defines weekly time ranges for checks or notifications
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

// Contact represents an alert recipient
type Contact struct {
	ID    string `yaml:"id" json:"id"`
	Email string `yaml:"email" json:"email"`
}

// HostGroup and ServiceGroup for organizational grouping
type HostGroup struct {
	ID    string `yaml:"id" json:"id"`
	Alias string `yaml:"alias" json:"alias"`
}

type ServiceGroup struct {
	ID    string `yaml:"id" json:"id"`
	Alias string `yaml:"alias" json:"alias"`
}

// Command represents the check execution logic
type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

// Host defines a monitored machine
type Host struct {
	ID                 string            `yaml:"id" json:"id"`
	Use                string            `yaml:"use" json:"use"`
	Address            string            `yaml:"address" json:"address"`
	CheckCommand       string            `yaml:"check_command" json:"check_command"`
	CheckPeriod        string            `yaml:"check_period" json:"check_period"`
	NotificationPeriod string            `yaml:"notification_period" json:"notification_period"`
	Alias              string            `yaml:"alias" json:"alias"`
	Contacts           []string          `yaml:"contacts" json:"contacts"`
	HostGroups         []string          `yaml:"hostgroups" json:"hostgroups"`
	Macros             map[string]string `yaml:"macros" json:"macros"`
	Register           *bool             `yaml:"register" json:"register"`
	InDowntime         bool              `json:"in_downtime"`
}

// Service defines a check performed on a Host
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
	InDowntime         bool              `json:"in_downtime"`
}

// GlobalConfig is the final payload sent from Arbiter to Scheduler
type GlobalConfig struct {
	Commands      []Command      `json:"commands"`
	Contacts      []Contact      `json:"contacts"`
	TimePeriods   []TimePeriod   `json:"timeperiods"`
	HostGroups    []HostGroup    `json:"hostgroups"`
	ServiceGroups []ServiceGroup `json:"servicegroups"`
	Hosts         []Host         `json:"hosts"`
	Services      []Service      `json:"services"`
	Downtimes     []Downtime     `json:"downtimes"`
}
