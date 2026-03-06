package models

import "time"

// GlobalConfig is the final payload sent to the Scheduler
type GlobalConfig struct {
	Commands      []Command      `json:"commands"`
	Contacts      []Contact      `json:"contacts"`
	TimePeriods   []TimePeriod   `json:"timeperiods"`
	Hosts         []Host         `json:"hosts"`
	Services      []Service      `json:"services"`
	HostGroups    []HostGroup    `json:"hostgroups"`
	ServiceGroups []ServiceGroup `json:"servicegroups"`
	Downtimes     []Downtime     `json:"downtimes"`
}

// Host represents a monitored machine or template
type Host struct {
	ID           string   `yaml:"id" json:"id"`
	Use          string   `yaml:"use" json:"use"` 
	Address      string   `yaml:"address" json:"address"`
	CheckCommand string   `yaml:"check_command" json:"check_command"`
	CheckPeriod  string   `yaml:"check_period" json:"check_period"`
	Contacts     []string `yaml:"contacts" json:"contacts"`
	HostGroups   []string `yaml:"hostgroups" json:"hostgroups"`
	Register     *bool    `yaml:"register" json:"register"` 
	InDowntime   bool     `json:"in_downtime"`
	Parents      []string `yaml:"parents" json:"parents"`
	// Runtime State Fields
	IsUp         bool      `json:"is_up"`     
	Status       int       `json:"status"`    
	NextCheck    time.Time `json:"next_check"`
	Output       string    `json:"output"`
}

// Service represents a specific check linked to a host
type Service struct {
	ID            string   `yaml:"id" json:"id"`
	Use           string   `yaml:"use" json:"use"` 
	HostName      string   `yaml:"host_name" json:"host_name"`
	CheckCommand  string   `yaml:"check_command" json:"check_command"`
	CheckPeriod   string   `yaml:"check_period" json:"check_period"`
	Contacts      []string `yaml:"contacts" json:"contacts"`
	ServiceGroups []string `yaml:"servicegroups" json:"servicegroups"`
	Register      *bool    `yaml:"register" json:"register"` 
	InDowntime    bool     `json:"in_downtime"`
	// Runtime State Fields
	CurrentState  int       `json:"current_state"`
	Attempts      int       `json:"attempts"`
	MaxAttempts   int       `json:"max_attempts"`
	NextCheck     time.Time `json:"next_check"`
	Output        string    `json:"output"`
}

// CheckResult is sent by Pollers to the Scheduler
type CheckResult struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Output string `json:"output"`
}

// NotificationRequest is sent to the Reactionner
type NotificationRequest struct {
	EntityID  string    `json:"entity_id"`
	Type      string    `json:"type"`
	State     int       `json:"state"`
	Output    string    `json:"output"`
	Timestamp time.Time `json:"timestamp"`
}

// Host group
type HostGroup struct {
	ID      string   `yaml:"id" json:"id"`
	Alias   string   `yaml:"alias" json:"alias"`
	Members []string `yaml:"members" json:"members"`
}

// Service group
type ServiceGroup struct {
	ID      string   `yaml:"id" json:"id"`
	Alias   string   `yaml:"alias" json:"alias"`
	Members []string `yaml:"members" json:"members"`
}

// Command defines the check execution string
type Command struct {
	ID          string `yaml:"id" json:"id"`
	CommandLine string `yaml:"command_line" json:"command_line"`
}

// Contact defines alert recipients
type Contact struct {
	ID       string `yaml:"id" json:"id"`
	Register *bool  `yaml:"register" json:"register"` 
	Email    string `yaml:"email" json:"email"`
}

// TimePeriod defines weekly time ranges
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

// Downtime represents a scheduled maintenance period
type Downtime struct {
	ID        string    `json:"id"`
	HostName  string    `json:"host_name"`
	ServiceID string    `json:"service_id,omitempty"` 
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Author    string    `json:"author"`
	Comment   string    `json:"comment"`
}

// CheckTask represents a single execution job for a Poller
type CheckTask struct {
	ID      string `json:"id"`      
	Command string `json:"command"` 
}
