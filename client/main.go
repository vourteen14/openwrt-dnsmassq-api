package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	version = "1.0.0"
	userAgent = "dnscli/" + version
)

type Config struct {
	Server string `json:"server"`
	APIKey string `json:"apikey"`
}

type Record struct {
	Domain string `json:"domain"`
	IP     string `json:"ip,omitempty"`
	NewIP  string `json:"new_ip,omitempty"`
}

type APIResponse struct {
	Records []Record `json:"records"`
	Status  string   `json:"status"`
	Error   string   `json:"error"`
	Domain  string   `json:"domain"`
	IP      string   `json:"ip"`
	NewIP   string   `json:"new_ip"`
}

var (
	flagSetup   = flag.Bool("setup", false, "configure server endpoint and API credentials")
	flagVersion = flag.Bool("version", false, "show version information")
	flagVerbose = flag.Bool("v", false, "enable verbose output")
	flagHelp    = flag.Bool("h", false, "show help")
	
	cmdList   = flag.Bool("list", false, "list all DNS records")
	cmdAdd    = flag.Bool("add", false, "add new DNS record")
	cmdUpdate = flag.Bool("update", false, "update existing DNS record")
	cmdDelete = flag.Bool("delete", false, "delete DNS record")
	
	optDomain = flag.String("domain", "", "target domain name")
	optIP     = flag.String("ip", "", "IP address")
	optNewIP  = flag.String("new-ip", "", "new IP address for update operation")
)

func init() {
	flag.Usage = showUsage
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dnscli", "config.json")
}

func loadConfig() (Config, error) {
	path := configPath()
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	
	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	dir := filepath.Dir(configPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	
	file, err := os.Create(configPath())
	if err != nil {
		return err
	}
	defer file.Close()
	
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}

func setupConfig() error {
	cfg, _ := loadConfig()
	
	fmt.Print("Server endpoint")
	if cfg.Server != "" {
		fmt.Printf(" [%s]", cfg.Server)
	}
	fmt.Print(": ")
	
	var input string
	fmt.Scanln(&input)
	if strings.TrimSpace(input) != "" {
		cfg.Server = strings.TrimSpace(input)
	}
	
	fmt.Print("API key")
	if cfg.APIKey != "" {
		fmt.Printf(" [%s]", cfg.APIKey[:8]+"...")
	}
	fmt.Print(": ")
	
	fmt.Scanln(&input)
	if strings.TrimSpace(input) != "" {
		cfg.APIKey = strings.TrimSpace(input)
	}
	
	if cfg.Server == "" {
		return fmt.Errorf("server endpoint is required")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	
	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %v", err)
	}
	
	fmt.Println("Configuration saved successfully")
	return nil
}

func formatOutput(responseBody []byte, isListCommand bool) {
	if *flagVerbose {
		var prettyJSON bytes.Buffer
		if json.Indent(&prettyJSON, responseBody, "", "  ") == nil {
			fmt.Print(prettyJSON.String())
		} else {
			fmt.Print(string(responseBody))
		}
		return
	}
	
	var resp APIResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		fmt.Print(string(responseBody))
		return
	}
	
	if resp.Error != "" {
		fmt.Printf("Error: %s\n", resp.Error)
		return
	}
	
	if isListCommand && len(resp.Records) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "DOMAIN\tIP ADDRESS\n")
		for _, record := range resp.Records {
			fmt.Fprintf(w, "%s\t%s\n", record.Domain, record.IP)
		}
		w.Flush()
		fmt.Printf("\nTotal: %d records\n", len(resp.Records))
	} else if isListCommand {
		fmt.Println("No DNS records found")
	} else {
		switch resp.Status {
		case "added":
			fmt.Printf("✓ Successfully added %s -> %s\n", resp.Domain, resp.IP)
		case "updated":
			fmt.Printf("✓ Successfully updated %s -> %s\n", resp.Domain, resp.NewIP)
		case "deleted":
			fmt.Printf("✓ Successfully deleted %s\n", resp.Domain)
		case "exists":
			fmt.Printf("Record already exists: %s -> %s\n", resp.Domain, resp.IP)
		default:
			fmt.Printf("Operation completed: %s\n", resp.Status)
		}
	}
}

func makeRequest(method, endpoint string, payload interface{}) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("configuration not found, run 'dnscli -setup' first")
	}
	
	url := strings.TrimSuffix(cfg.Server, "/") + endpoint
	isListCommand := method == "GET" && endpoint == "/dns"
	
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %v", err)
		}
		body = bytes.NewReader(data)
		
		if *flagVerbose {
			fmt.Fprintf(os.Stderr, "> %s %s\n", method, url)
			fmt.Fprintf(os.Stderr, "> Content-Type: application/json\n")
			fmt.Fprintf(os.Stderr, "> X-API-Key: %s\n", cfg.APIKey[:8]+"...")
			fmt.Fprintf(os.Stderr, ">\n%s\n", string(data))
		}
	} else if *flagVerbose {
		fmt.Fprintf(os.Stderr, "> %s %s\n", method, url)
		fmt.Fprintf(os.Stderr, "> X-API-Key: %s\n", cfg.APIKey[:8]+"...")
	}
	
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", cfg.APIKey)
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	if *flagVerbose {
		fmt.Fprintf(os.Stderr, "< HTTP/%s %s\n", resp.Proto[5:], resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(os.Stderr, "< %s: %s\n", k, strings.Join(v, ", "))
		}
		fmt.Fprintf(os.Stderr, "<\n")
	}
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s: %s", resp.Status, string(responseBody))
	}
	
	formatOutput(responseBody, isListCommand)
	return nil
}

func showUsage() {
	fmt.Printf(`dnscli - DNS record management client %s

USAGE:
    dnscli [OPTIONS] COMMAND [ARGS]

OPTIONS:
    -h, --help      show this help message
    -v, --verbose   enable verbose output
    --version       show version information
    --setup         configure server endpoint and credentials

COMMANDS:
    --list                                  list all DNS records
    --add --domain <name> --ip <addr>       add new DNS record
    --update --domain <name> [--ip <old>] --new-ip <addr>
                                            update existing DNS record
    --delete --domain <name> [--ip <addr>]  delete DNS record

EXAMPLES:
    dnscli --setup
    dnscli --list
    dnscli --add --domain api.example.com --ip 192.168.1.100
    dnscli --update --domain api.example.com --new-ip 192.168.1.101
    dnscli --delete --domain api.example.com

For more information, see the documentation.
`, version)
}

func validateArgs() error {
	commands := 0
	if *cmdList { commands++ }
	if *cmdAdd { commands++ }
	if *cmdUpdate { commands++ }
	if *cmdDelete { commands++ }
	
	if commands == 0 {
		return fmt.Errorf("no command specified")
	}
	if commands > 1 {
		return fmt.Errorf("multiple commands specified")
	}
	
	if *cmdAdd {
		if *optDomain == "" || *optIP == "" {
			return fmt.Errorf("add command requires --domain and --ip")
		}
	}
	
	if *cmdUpdate {
		if *optDomain == "" || *optNewIP == "" {
			return fmt.Errorf("update command requires --domain and --new-ip")
		}
	}
	
	if *cmdDelete {
		if *optDomain == "" {
			return fmt.Errorf("delete command requires --domain")
		}
	}
	
	return nil
}

func main() {
	flag.Parse()
	
	if *flagHelp {
		showUsage()
		return
	}
	
	if *flagVersion {
		fmt.Printf("dnscli version %s\n", version)
		return
	}
	
	if *flagSetup {
		if err := setupConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "dnscli: %v\n", err)
			os.Exit(1)
		}
		return
	}
	
	if err := validateArgs(); err != nil {
		fmt.Fprintf(os.Stderr, "dnscli: %v\n", err)
		fmt.Fprintf(os.Stderr, "Try 'dnscli --help' for more information.\n")
		os.Exit(1)
	}
	
	var err error
	
	switch {
	case *cmdList:
		err = makeRequest("GET", "/dns", nil)
		
	case *cmdAdd:
		payload := Record{Domain: *optDomain, IP: *optIP}
		err = makeRequest("POST", "/dns", payload)
		
	case *cmdUpdate:
		payload := Record{Domain: *optDomain, IP: *optIP, NewIP: *optNewIP}
		err = makeRequest("PUT", "/dns", payload)
		
	case *cmdDelete:
		payload := Record{Domain: *optDomain, IP: *optIP}
		err = makeRequest("DELETE", "/dns", payload)
	}
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "dnscli: %v\n", err)
		os.Exit(1)
	}
}