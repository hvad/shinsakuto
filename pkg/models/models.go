package models

import "time"

// Downtime represents a scheduled maintenance window
type Downtime struct {
	ID        string    `json:"id"`
	HostName  string    `json:"host_name"`
	ServiceID string    `json:"service_id"` 
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Author    string    `json:"author"`
	Comment   string    `json:"comment"`
}

// TimePeriod defines weekly time ranges for checks or notifications
type TimePeriod struct {
	ID        string   `yaml:"id" json:"id"`
	Register  *bool    `yaml:"register" json:"register"` 
	Monday    []string `yaml:"monday" json:"monday"`
	Tuesday   []string `yaml:"tuesday" json:"tuesday"`
	Wednesday []string `yaml:"wednesday" json:"wednesday"`
	Thursday  []string `yaml:"thursday" json:"thursday"`
	Friday    []string `yaml:"friday" json:"friday"`
	Saturday  []string `yaml:"saturday" json:"saturday"`
	Sunday    []string `yaml:"sunday" json:"sunday"`
}

// Contact defines an alert recipient
type Contact struct {
	ID       string `yaml:"id" json:"id"`
	Register *bool  `yaml:"register" json:"register"` 
	Email    string `yaml:"email" json:"email"`
}

// Command defines the execution logic for checks
type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

// Host represents a monitored machine or a template
type Host struct {
	ID           string   `yaml:"id" json:"id"`
	Use          string   `yaml:"use" json:"use"` 
	Address      string   `yaml:"address" json:"address"`
	CheckCommand string   `yaml:"check_command" json:"check_command"`
	CheckPeriod  string   `yaml:"check_period" json:"check_period"`
	Contacts     []string `yaml:"contacts" json:"contacts"`
	Register     *bool    `yaml:"register" json:"register"` 
	InDowntime   bool     `json:"in_downtime"`             
}

// Service represents a specific check bound to a host
type Service struct {
	ID           string   `yaml:"id" json:"id"`
	Use          string   `yaml:"use" json:"use"` 
	HostName     string   `yaml:"host_name" json:"host_name"`
	CheckCommand string   `yaml:"check_command" json:"check_command"`
	CheckPeriod  string   `yaml:"check_period" json:"check_period"`
	Contacts     []string `yaml:"contacts" json:"contacts"`
	Register     *bool    `yaml:"register" json:"register"` 
	InDowntime   bool     `json:"in_downtime"`             
}

// GlobalConfig is the final payload pushed to the Scheduler
type GlobalConfig struct {
	Commands    []Command    `json:"commands"`
	Contacts    []Contact    `json:"contacts"`
	TimePeriods []TimePeriod `json:"timeperiods"`
	Hosts       []Host       `json:"hosts"`
	Services    []Service    `json:"services"`
	Downtimes   []Downtime   `json:"downtimes"`
}
