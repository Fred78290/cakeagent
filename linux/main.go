package main

import (
	"encoding/json"
	"fmt"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/pkg"
	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/mount"
	"github.com/Fred78290/cakeagent/service"
	"github.com/Fred78290/cakeagent/version"

	glog "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var cfg *types.Config
var jsonOutput bool
var pingJsonOutput bool
var mountJsonOutput bool

var rootCmd = &cobra.Command{
	Use:   "cakeagent",
	Short: "Cake agent running in guest vm",
	Long:  "Cake agent running in guest vm.\n\nNote that all flags may be replaced with env vars prefixed with CAKEAGENT_",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("The current version is: %s, build at: %s\n", version.VERSION, version.BUILD_DATE)
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the cakeagent server",
	Long:  "Start the cakeagent server and listen for incoming connections",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		startServer()
	},
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage cakeagent service",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install cakeagent as a system service",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		if err := service.InstallService(cfg); err != nil {
			glog.Fatalf("failed to install service: %v", err)
		}
		glog.Info("Service installed successfully")
	},
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the cakeagent service",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		if err := service.StartService(cfg); err != nil {
			glog.Fatalf("failed to start service: %v", err)
		}
		glog.Info("Service started successfully")
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the cakeagent service",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		if err := service.StopService(cfg); err != nil {
			glog.Fatalf("failed to stop service: %v", err)
		}
		glog.Info("Service stopped successfully")
	},
}

var infosCmd = &cobra.Command{
	Use:   "infos",
	Short: "Display system information",
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		displaySystemInfo(jsonOutput)
	},
}

var pingCmd = &cobra.Command{
	Use:   "ping [message]",
	Short: "Ping the cakeagent service",
	Long:  "Send a ping message to the cakeagent service and receive a pong response",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		message := "hello"
		if len(args) > 0 {
			message = args[0]
		}
		pingService(message, pingJsonOutput)
	},
}

var mountCmd = &cobra.Command{
	Use:   "mount [mount-string...]",
	Short: "Mount VirtioFS endpoints",
	Long: `Mount VirtioFS endpoints using the cakeagent service.

Mount strings should be in the format: name:target[,uid=X,gid=Y,ro,early]

Examples:
  cakeagent mount share:/mnt/share
  cakeagent mount data:/data,uid=1000,gid=1000
  cakeagent mount logs:/var/log,uid=0,gid=0,ro
  cakeagent mount config:/etc/config,ro,early share:/mnt/share,uid=1000
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		setupLogging()
		mountServiceWithArgs(args, mountJsonOutput)
	},
}

func setupLogging() {
	ll, err := glog.ParseLevel(cfg.LogLevel)
	if err != nil {
		glog.Fatalf("failed to parse log level: %v", err)
	}

	glog.SetLevel(ll)

	if cfg.LogFormat == "json" {
		glog.SetFormatter(&glog.JSONFormatter{})
	}
}

func startServer() {
	glog.Infof("config: %s", cfg)

	var err error
	if len(cfg.Mounts) > 0 {
		err = mount.MountEndpoints(cfg.Mounts)
	}

	if err == nil {
		_, err = pkg.StartServer(cfg)
	}

	if err != nil {
		glog.Fatalf("%v", err)
	}
}

func displaySystemInfo(jsonFormat bool) {
	info, err := pkg.GetSystemInfo()
	if err != nil {
		glog.Fatalf("failed to get system info: %v", err)
		return
	}

	if jsonFormat {
		displaySystemInfoJSON(info)
	} else {
		displaySystemInfoText(info)
	}
}

func displaySystemInfoJSON(info *cakeagent.CakeAgent_InfoReply) {
	// Convert protobuf to JSON
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		glog.Fatalf("failed to marshal to JSON: %v", err)
		return
	}
	fmt.Println(string(jsonData))
}

func displaySystemInfoText(info *cakeagent.CakeAgent_InfoReply) {
	// Display system information in a formatted way
	fmt.Println("=== System Information ===")
	fmt.Printf("Agent Version: %s\n", info.AgentVersion)
	fmt.Printf("Hostname: %s\n", info.Hostname)
	fmt.Printf("OS Name: %s\n", info.Osname)
	fmt.Printf("OS Release: %s\n", info.Release)
	fmt.Printf("OS Version: %s\n", info.Version)
	fmt.Printf("Uptime: %d nanoseconds\n", info.Uptime)

	fmt.Println("\n=== CPU Information ===")
	fmt.Printf("CPU Count: %d\n", info.CpuCount)
	if info.Cpu != nil {
		fmt.Printf("Total CPU Usage: %.2f%%\n", info.Cpu.TotalUsagePercent)
		fmt.Printf("User Time: %.2f\n", info.Cpu.User)
		fmt.Printf("System Time: %.2f\n", info.Cpu.System)
		fmt.Printf("Idle Time: %.2f\n", info.Cpu.Idle)
		fmt.Printf("IO Wait: %.2f\n", info.Cpu.Iowait)

		if len(info.Cpu.Cores) > 0 {
			fmt.Println("\n--- Per-Core CPU Usage ---")
			for _, core := range info.Cpu.Cores {
				fmt.Printf("Core %d: %.2f%% usage\n", core.CoreId, core.UsagePercent)
			}
		}
	}

	fmt.Println("\n=== Memory Information ===")
	if info.Memory != nil {
		fmt.Printf("Total Memory: %d bytes (%.2f GB)\n", info.Memory.Total, float64(info.Memory.Total)/1024/1024/1024)
		fmt.Printf("Free Memory: %d bytes (%.2f GB)\n", info.Memory.Free, float64(info.Memory.Free)/1024/1024/1024)
		fmt.Printf("Used Memory: %d bytes (%.2f GB)\n", info.Memory.Used, float64(info.Memory.Used)/1024/1024/1024)
		fmt.Printf("Memory Usage: %.2f%%\n", float64(info.Memory.Used)/float64(info.Memory.Total)*100)
	}

	fmt.Println("\n=== Disk Information ===")
	for _, disk := range info.DiskInfos {
		fmt.Printf("Device: %s\n", disk.Device)
		fmt.Printf("  Mount: %s\n", disk.Mount)
		fmt.Printf("  Type: %s\n", disk.FsType)
		fmt.Printf("  Total: %d bytes (%.2f GB)\n", disk.Size, float64(disk.Size)/1024/1024/1024)
		fmt.Printf("  Free: %d bytes (%.2f GB)\n", disk.Free, float64(disk.Free)/1024/1024/1024)
		fmt.Printf("  Used: %d bytes (%.2f GB)\n", disk.Used, float64(disk.Used)/1024/1024/1024)
		fmt.Printf("  Usage: %.2f%%\n", float64(disk.Used)/float64(disk.Size)*100)
		fmt.Println()
	}

	fmt.Println("=== Network Information ===")
	for i, ip := range info.Ipaddresses {
		fmt.Printf("IP Address %d: %s\n", i+1, ip)
	}
}

func pingService(message string, jsonFormat bool) {
	result, err := pkg.PingService(message)
	if err != nil {
		glog.Fatalf("failed to ping service: %v", err)
		return
	}

	if jsonFormat {
		pingOutputJSON(result)
	} else {
		pingOutputText(result)
	}
}

func pingOutputJSON(result *cakeagent.CakeAgent_PingReply) {
	// Create a structured response for JSON output
	response := map[string]interface{}{
		"message":            result.Message,
		"request_timestamp":  result.RequestTimestamp,
		"response_timestamp": result.ResponseTimestamp,
		"round_trip_time_ns": result.ResponseTimestamp - result.RequestTimestamp,
		"round_trip_time_ms": float64(result.ResponseTimestamp-result.RequestTimestamp) / 1000000.0,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		glog.Fatalf("failed to marshal ping response to JSON: %v", err)
		return
	}
	fmt.Println(string(jsonData))
}

func pingOutputText(result *cakeagent.CakeAgent_PingReply) {
	duration := result.ResponseTimestamp - result.RequestTimestamp
	fmt.Printf("PONG: %s\n", result.Message)
	fmt.Printf("Request sent at: %d\n", result.RequestTimestamp)
	fmt.Printf("Response received at: %d\n", result.ResponseTimestamp)
	fmt.Printf("Round trip time: %d nanoseconds (%.2f ms)\n", duration, float64(duration)/1000000.0)
}

func mountServiceWithArgs(mountStrings []string, jsonFormat bool) {
	if len(mountStrings) == 0 {
		if jsonFormat {
			response := map[string]interface{}{
				"error": "No mount strings provided",
			}
			jsonData, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(jsonData))
		} else {
			fmt.Println("No mount strings provided")
		}
		return
	}

	// Parse mount strings using the new parsing function
	mounts, err := pkg.ParseMountStrings(mountStrings, true)
	if err != nil {
		if jsonFormat {
			response := map[string]interface{}{
				"error": fmt.Sprintf("Failed to parse mount strings: %v", err),
			}
			jsonData, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(jsonData))
		} else {
			glog.Fatalf("Failed to parse mount strings: %v", err)
		}
		return
	}

	result, err := pkg.MountService(mounts)
	if err != nil {
		if jsonFormat {
			response := map[string]interface{}{
				"error": err.Error(),
			}
			jsonData, _ := json.MarshalIndent(response, "", "  ")
			fmt.Println(string(jsonData))
		} else {
			glog.Fatalf("failed to mount: %v", err)
		}
		return
	}

	if jsonFormat {
		mountOutputJSON(result)
	} else {
		mountOutputText(result)
	}
}

func mountOutputJSON(result *cakeagent.CakeAgent_MountReply) {
	// Convert to a more readable JSON structure
	response := map[string]interface{}{}

	if result.GetError() != "" {
		response["error"] = result.GetError()
		response["success"] = false
	} else {
		response["success"] = result.GetSuccess()
	}

	mounts := make([]map[string]interface{}, 0)
	for _, mount := range result.GetMounts() {
		mountInfo := map[string]interface{}{
			"name": mount.GetName(),
		}
		if mount.GetError() != "" {
			mountInfo["error"] = mount.GetError()
			mountInfo["success"] = false
		} else {
			mountInfo["success"] = mount.GetSuccess()
		}
		mounts = append(mounts, mountInfo)
	}
	response["mounts"] = mounts

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		glog.Fatalf("failed to marshal mount response to JSON: %v", err)
		return
	}
	fmt.Println(string(jsonData))
}

func mountOutputText(result *cakeagent.CakeAgent_MountReply) {
	if result.GetError() != "" {
		fmt.Printf("Mount operation failed: %s\n", result.GetError())
		return
	}

	if result.GetSuccess() {
		fmt.Println("Mount operation completed successfully")
	} else {
		fmt.Println("Mount operation completed with errors")
	}

	for _, mount := range result.GetMounts() {
		if mount.GetError() != "" {
			fmt.Printf("  - %s: FAILED - %s\n", mount.GetName(), mount.GetError())
		} else if mount.GetSuccess() {
			fmt.Printf("  - %s: SUCCESS\n", mount.GetName())
		} else {
			fmt.Printf("  - %s: FAILED - unknown error\n", mount.GetName())
		}
	}
}

func init() {
	cfg = types.NewConfig()

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfg.LogLevel, "log-level", "l", cfg.LogLevel, "Set the level of logging (panic, debug, info, warning, error, fatal)")
	rootCmd.PersistentFlags().StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "The format in which log messages are printed (text, json)")

	// Run command flags
	serveCmd.Flags().StringVar(&cfg.Address, "listen", cfg.Address, "Listen on address")
	serveCmd.Flags().StringVar(&cfg.CaCert, "ca-cert", cfg.CaCert, "CA TLS certificate")
	serveCmd.Flags().StringVar(&cfg.TlsCert, "tls-cert", cfg.TlsCert, "Server TLS certificate")
	serveCmd.Flags().StringVar(&cfg.TlsKey, "tls-key", cfg.TlsKey, "Server private key")
	serveCmd.Flags().DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "Request timeout for operation. 0s means no timeout")
	serveCmd.Flags().DurationVar(&cfg.TickEvent, "tick", cfg.TickEvent, "Tick event idling")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(infosCmd)
	rootCmd.AddCommand(pingCmd)
	rootCmd.AddCommand(mountCmd)

	// Add service subcommands
	serviceInstallCmd.Flags().StringVar(&cfg.Address, "listen", cfg.Address, "Listen on address")
	serviceInstallCmd.Flags().StringVar(&cfg.CaCert, "ca-cert", cfg.CaCert, "CA TLS certificate")
	serviceInstallCmd.Flags().StringVar(&cfg.TlsCert, "tls-cert", cfg.TlsCert, "Server TLS certificate")
	serviceInstallCmd.Flags().StringVar(&cfg.TlsKey, "tls-key", cfg.TlsKey, "Server private key")
	serviceInstallCmd.Flags().DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "Request timeout for operation. 0s means no timeout")
	serviceInstallCmd.Flags().DurationVar(&cfg.TickEvent, "tick", cfg.TickEvent, "Tick event idling")
	serviceInstallCmd.Flags().StringSliceVar(&cfg.Mounts, "mount", cfg.Mounts, "Mount endpoint")

	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)

	// Add infos command flags
	infosCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output system information in JSON format")

	// Add ping command flags
	pingCmd.Flags().BoolVar(&pingJsonOutput, "json", false, "Output ping response in JSON format")

	// Add mount command flags
	mountCmd.Flags().BoolVar(&mountJsonOutput, "json", false, "Output mount response in JSON format")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		glog.Fatalf("%v", err)
	}
}
