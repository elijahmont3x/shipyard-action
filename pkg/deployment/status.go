package deployment

import (
    "fmt"
    "os"
    "strings"
    "time"
)

// StatusReporter handles reporting status back to GitHub Actions
type StatusReporter struct {
    enabled bool
}

// NewStatusReporter creates a new status reporter
func NewStatusReporter() *StatusReporter {
    // Check if running in GitHub Actions
    enabled := os.Getenv("GITHUB_ACTIONS") == "true"
    
    return &StatusReporter{
        enabled: enabled,
    }
}

// StartGroup starts a collapsible group in GitHub Actions logs
func (r *StatusReporter) StartGroup(name string) {
    if r.enabled {
        fmt.Printf("::group::%s\n", name)
    } else {
        fmt.Printf("=== %s ===\n", name)
    }
}

// EndGroup ends a collapsible group
func (r *StatusReporter) EndGroup() {
    if r.enabled {
        fmt.Println("::endgroup::")
    } else {
        fmt.Println("===========")
    }
}

// SetOutput sets an output variable for GitHub Actions
func (r *StatusReporter) SetOutput(name, value string) {
    if r.enabled {
        // Escape special characters
        value = strings.ReplaceAll(value, "%", "%25")
        value = strings.ReplaceAll(value, "\r", "%0D")
        value = strings.ReplaceAll(value, "\n", "%0A")
        
        fmt.Printf("::set-output name=%s::%s\n", name, value)
    }
}

// LogDebug logs a debug message
func (r *StatusReporter) LogDebug(message string) {
    if r.enabled {
        fmt.Printf("::debug::%s\n", message)
    } else {
        fmt.Printf("[DEBUG] %s\n", message)
    }
}

// LogWarning logs a warning message
func (r *StatusReporter) LogWarning(message string) {
    if r.enabled {
        fmt.Printf("::warning::%s\n", message)
    } else {
        fmt.Printf("[WARNING] %s\n", message)
    }
}

// LogError logs an error message
func (r *StatusReporter) LogError(message string) {
    if r.enabled {
        fmt.Printf("::error::%s\n", message)
    } else {
        fmt.Printf("[ERROR] %s\n", message)
    }
}

// StartTimer starts a timer for a step
func (r *StatusReporter) StartTimer(name string) *Timer {
    return &Timer{
        name:      name,
        startTime: time.Now(),
        reporter:  r,
    }
}

// Timer tracks the execution time of a step
type Timer struct {
    name      string
    startTime time.Time
    reporter  *StatusReporter
}

// Stop stops the timer and logs the duration
func (t *Timer) Stop() {
    duration := time.Since(t.startTime)
    
    message := fmt.Sprintf("%s completed in %s", t.name, duration)
    t.reporter.LogDebug(message)
}