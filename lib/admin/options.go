package admin

// Options holds admin user auto-creation configuration
type Options struct {
	AutocreateAdmin bool   `long:"autocreate-admin" env:"AUTOCREATE_ADMIN" ini-name:"autocreate-admin" description:"Automatically create admin user on startup if it doesn't exist"`
	AdminUsername   string `long:"admin-username" env:"ADMIN_USERNAME" ini-name:"admin-username" description:"Username for auto-created admin user" default:"admin"`
	AdminEmail      string `long:"admin-email" env:"ADMIN_EMAIL" ini-name:"admin-email" description:"Email for auto-created admin user"`
	AdminAPIKey     string `long:"admin-apikey" env:"ADMIN_APIKEY" ini-name:"admin-apikey" description:"API key for auto-created admin user (generated if not provided)"`
}

// NewOptions creates a new Options with defaults
func NewOptions() *Options {
	return &Options{
		AdminUsername: "admin",
	}
}

// Enabled returns true if admin auto-creation is enabled
func (o *Options) Enabled() bool {
	return o.AutocreateAdmin
}
