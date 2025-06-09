package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcphost/internal/agent"
	"github.com/mark3labs/mcphost/internal/config"
	"github.com/mark3labs/mcphost/internal/models"
	"github.com/mark3labs/mcphost/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	configFile       string
	systemPromptFile string
	messageWindow    int
	modelFlag        string
	openaiBaseURL    string
	anthropicBaseURL string
	openaiAPIKey     string
	anthropicAPIKey  string
	googleAPIKey     string
	debugMode        bool
	promptFlag       string
	quietFlag        bool
	scriptFlag       bool
	maxSteps         int
	scriptMCPConfig  *config.Config // Used to override config in script mode
)

var rootCmd = &cobra.Command{
	Use:   "mcphost",
	Short: "Chat with AI models through a unified interface",
	Long: `MCPHost is a CLI tool that allows you to interact with various AI models
through a unified interface. It supports various tools through MCP servers
and provides streaming responses.

Available models can be specified using the --model flag:
- Anthropic Claude (default): anthropic:claude-sonnet-4-20250514
- OpenAI: openai:gpt-4
- Ollama models: ollama:modelname
- Google: google:modelname

Examples:
  # Interactive mode
  mcphost -m ollama:qwen2.5:3b
  mcphost -m openai:gpt-4
  mcphost -m google:gemini-2.0-flash
  
  # Non-interactive mode
  mcphost -p "What is the weather like today?"
  mcphost -p "Calculate 15 * 23" --quiet
  
  # Script mode
  mcphost --script myscript.sh
  ./myscript.sh  # if script has shebang #!/path/to/mcphost --script`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPHost(context.Background())
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().
		StringVar(&configFile, "config", "", "config file (default is $HOME/.mcp.json)")
	rootCmd.PersistentFlags().
		StringVar(&systemPromptFile, "system-prompt", "", "system prompt json file")
	rootCmd.PersistentFlags().
		IntVar(&messageWindow, "message-window", 40, "number of messages to keep in context")
	rootCmd.PersistentFlags().
		StringVarP(&modelFlag, "model", "m", "anthropic:claude-sonnet-4-20250514",
			"model to use (format: provider:model)")
	rootCmd.PersistentFlags().
		BoolVar(&debugMode, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().
		StringVarP(&promptFlag, "prompt", "p", "", "run in non-interactive mode with the given prompt")
	rootCmd.PersistentFlags().
		BoolVar(&quietFlag, "quiet", false, "suppress all output (only works with --prompt)")
	rootCmd.PersistentFlags().
		BoolVar(&scriptFlag, "script", false, "run in script mode (parse YAML frontmatter and prompt from file)")
	rootCmd.PersistentFlags().
		IntVar(&maxSteps, "max-steps", 0, "maximum number of agent steps (0 for unlimited)")

	flags := rootCmd.PersistentFlags()
	flags.StringVar(&openaiBaseURL, "openai-url", "", "base URL for OpenAI API")
	flags.StringVar(&anthropicBaseURL, "anthropic-url", "", "base URL for Anthropic API")
	flags.StringVar(&openaiAPIKey, "openai-api-key", "", "OpenAI API key")
	flags.StringVar(&anthropicAPIKey, "anthropic-api-key", "", "Anthropic API key")
	flags.StringVar(&googleAPIKey, "google-api-key", "", "Google (Gemini) API key")

	// Bind flags to viper for config file support
	viper.BindPFlag("system-prompt", rootCmd.PersistentFlags().Lookup("system-prompt"))
	viper.BindPFlag("message-window", rootCmd.PersistentFlags().Lookup("message-window"))
	viper.BindPFlag("model", rootCmd.PersistentFlags().Lookup("model"))
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("max-steps", rootCmd.PersistentFlags().Lookup("max-steps"))
	viper.BindPFlag("openai-url", rootCmd.PersistentFlags().Lookup("openai-url"))
	viper.BindPFlag("anthropic-url", rootCmd.PersistentFlags().Lookup("anthropic-url"))
	viper.BindPFlag("openai-api-key", rootCmd.PersistentFlags().Lookup("openai-api-key"))
	viper.BindPFlag("anthropic-api-key", rootCmd.PersistentFlags().Lookup("anthropic-api-key"))
	viper.BindPFlag("google-api-key", rootCmd.PersistentFlags().Lookup("google-api-key"))
}

func runMCPHost(ctx context.Context) error {
	// Handle script mode
	if scriptFlag {
		return runScriptMode(ctx)
	}

	return runNormalMode(ctx)
}

func runNormalMode(ctx context.Context) error {
	// Validate flag combinations
	if quietFlag && promptFlag == "" {
		return fmt.Errorf("--quiet flag can only be used with --prompt/-p")
	}

	// Set up logging
	if debugMode {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Load configuration
	var mcpConfig *config.Config
	var err error
	
	if scriptMCPConfig != nil {
		// Use script-provided config
		mcpConfig = scriptMCPConfig
	} else {
		// Load normal config
		mcpConfig, err = config.LoadMCPConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load MCP config: %v", err)
		}
	}

	// Set up viper to read from the same config file for flag values
	if configFile == "" {
		// Use default config file locations
		homeDir, err := os.UserHomeDir()
		if err == nil {
			viper.SetConfigName(".mcphost")
			viper.AddConfigPath(homeDir)
			viper.SetConfigType("yaml")
			if err := viper.ReadInConfig(); err != nil {
				// Try .mcphost.json
				viper.SetConfigType("json")
				if err := viper.ReadInConfig(); err != nil {
					// Try legacy .mcp files
					viper.SetConfigName(".mcp")
					viper.SetConfigType("yaml")
					if err := viper.ReadInConfig(); err != nil {
						viper.SetConfigType("json")
						viper.ReadInConfig() // Ignore error if no config found
					}
				}
			}
		}
	} else {
		// Use specified config file
		viper.SetConfigFile(configFile)
		viper.ReadInConfig() // Ignore error if file doesn't exist
	}

	// Override flag values with config file values (using viper's bound values)
	if viper.GetString("system-prompt") != "" {
		systemPromptFile = viper.GetString("system-prompt")
	}
	if viper.GetInt("message-window") != 0 {
		messageWindow = viper.GetInt("message-window")
	}
	if viper.GetString("model") != "" {
		modelFlag = viper.GetString("model")
	}
	if viper.GetBool("debug") {
		debugMode = viper.GetBool("debug")
	}
	if viper.GetInt("max-steps") != 0 {
		maxSteps = viper.GetInt("max-steps")
	}
	if viper.GetString("openai-url") != "" {
		openaiBaseURL = viper.GetString("openai-url")
	}
	if viper.GetString("anthropic-url") != "" {
		anthropicBaseURL = viper.GetString("anthropic-url")
	}
	if viper.GetString("openai-api-key") != "" {
		openaiAPIKey = viper.GetString("openai-api-key")
	}
	if viper.GetString("anthropic-api-key") != "" {
		anthropicAPIKey = viper.GetString("anthropic-api-key")
	}
	if viper.GetString("google-api-key") != "" {
		googleAPIKey = viper.GetString("google-api-key")
	}

	systemPrompt, err := config.LoadSystemPrompt(systemPromptFile)
	if err != nil {
		return fmt.Errorf("failed to load system prompt: %v", err)
	}

	// Create model configuration
	modelConfig := &models.ProviderConfig{
		ModelString:      modelFlag,
		SystemPrompt:     systemPrompt,
		AnthropicAPIKey:  anthropicAPIKey,
		AnthropicBaseURL: anthropicBaseURL,
		OpenAIAPIKey:     openaiAPIKey,
		OpenAIBaseURL:    openaiBaseURL,
		GoogleAPIKey:     googleAPIKey,
	}

	// Create agent configuration
	agentMaxSteps := maxSteps
	if agentMaxSteps == 0 {
		agentMaxSteps = 1000 // Set a high limit for "unlimited"
	}
	
	agentConfig := &agent.AgentConfig{
		ModelConfig:   modelConfig,
		MCPConfig:     mcpConfig,
		SystemPrompt:  systemPrompt,
		MaxSteps:      agentMaxSteps,
		MessageWindow: messageWindow,
	}

	// Create the agent
	mcpAgent, err := agent.NewAgent(ctx, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to create agent: %v", err)
	}
	defer mcpAgent.Close()

	// Get model name for display
	parts := strings.SplitN(modelFlag, ":", 2)
	modelName := "Unknown"
	if len(parts) == 2 {
		modelName = parts[1]
	}

	// Get tools
	tools := mcpAgent.GetTools()

	// Create CLI interface (skip if quiet mode)
	var cli *ui.CLI
	if !quietFlag {
		cli, err = ui.NewCLI()
		if err != nil {
			return fmt.Errorf("failed to create CLI: %v", err)
		}

		// Log successful initialization
		if len(parts) == 2 {
			cli.DisplayInfo(fmt.Sprintf("Model loaded: %s (%s)", parts[0], parts[1]))
		}
		cli.DisplayInfo(fmt.Sprintf("Loaded %d tools from MCP servers", len(tools)))
	}

	// Prepare data for slash commands
	var serverNames []string
	for name := range mcpConfig.MCPServers {
		serverNames = append(serverNames, name)
	}

	var toolNames []string
	for _, tool := range tools {
		if info, err := tool.Info(ctx); err == nil {
			toolNames = append(toolNames, info.Name)
		}
	}

	// Main interaction logic
	var messages []*schema.Message
	
	// Check if running in non-interactive mode
	if promptFlag != "" {
		return runNonInteractiveMode(ctx, mcpAgent, cli, promptFlag, modelName, messages, quietFlag)
	}
	
	// Quiet mode is not allowed in interactive mode
	if quietFlag {
		return fmt.Errorf("--quiet flag can only be used with --prompt/-p")
	}
	
	return runInteractiveMode(ctx, mcpAgent, cli, serverNames, toolNames, modelName, messages)
}

// runNonInteractiveMode handles the non-interactive mode execution
func runNonInteractiveMode(ctx context.Context, mcpAgent *agent.Agent, cli *ui.CLI, prompt, modelName string, messages []*schema.Message, quiet bool) error {
	// Display user message (skip if quiet)
	if !quiet && cli != nil {
		cli.DisplayUserMessage(prompt)
	}

	// Add user message to history
	messages = append(messages, schema.UserMessage(prompt))

	// Get agent response with controlled spinner that stops for tool call display
	var response *schema.Message
	var currentSpinner *ui.Spinner
	
	// Start initial spinner (skip if quiet)
	if !quiet && cli != nil {
		currentSpinner = ui.NewSpinner("Thinking...")
		currentSpinner.Start()
	}
	
	response, err := mcpAgent.GenerateWithLoop(ctx, messages,
		// Tool call handler - called when a tool is about to be executed
		func(toolName, toolArgs string) {
			if !quiet && cli != nil {
				// Stop spinner before displaying tool call
				if currentSpinner != nil {
					currentSpinner.Stop()
					currentSpinner = nil
				}
				cli.DisplayToolCallMessage(toolName, toolArgs)
			}
		},
		// Tool execution handler - called when tool execution starts/ends
		func(toolName string, isStarting bool) {
			if !quiet && cli != nil {
				if isStarting {
					// Start spinner for tool execution
					currentSpinner = ui.NewSpinner(fmt.Sprintf("Executing %s...", toolName))
					currentSpinner.Start()
				} else {
					// Stop spinner when tool execution completes
					if currentSpinner != nil {
						currentSpinner.Stop()
						currentSpinner = nil
					}
				}
			}
		},
		// Tool result handler - called when a tool execution completes
		func(toolName, toolArgs, result string, isError bool) {
			if !quiet && cli != nil {
				cli.DisplayToolMessage(toolName, toolArgs, result, isError)
				// Start spinner again for next LLM call
				currentSpinner = ui.NewSpinner("Thinking...")
				currentSpinner.Start()
			}
		},
		// Response handler - called when the LLM generates a response
		func(content string) {
			if !quiet && cli != nil {
				// Stop spinner when we get the final response
				if currentSpinner != nil {
					currentSpinner.Stop()
					currentSpinner = nil
				}
			}
		},

		// Tool call content handler - called when content accompanies tool calls
		func(content string) {
			if !quiet && cli != nil {
				// Stop spinner before displaying content
				if currentSpinner != nil {
					currentSpinner.Stop()
					currentSpinner = nil
				}
				cli.DisplayAssistantMessageWithModel(content, modelName)
				// Start spinner again for tool calls
				currentSpinner = ui.NewSpinner("Thinking...")
				currentSpinner.Start()
			}
		},
	)
	
	// Make sure spinner is stopped if still running
	if !quiet && cli != nil && currentSpinner != nil {
		currentSpinner.Stop()
	}
	if err != nil {
		if !quiet && cli != nil {
			cli.DisplayError(fmt.Errorf("agent error: %v", err))
		}
		return err
	}

	// Display assistant response with model name (skip if quiet)
	if !quiet && cli != nil {
		if err := cli.DisplayAssistantMessageWithModel(response.Content, modelName); err != nil {
			cli.DisplayError(fmt.Errorf("display error: %v", err))
			return err
		}
	} else if quiet {
		// In quiet mode, only output the final response content to stdout
		fmt.Print(response.Content)
	}

	// Exit after displaying the final response
	return nil
}

// runInteractiveMode handles the interactive mode execution
func runInteractiveMode(ctx context.Context, mcpAgent *agent.Agent, cli *ui.CLI, serverNames, toolNames []string, modelName string, messages []*schema.Message) error {

	// Main interaction loop
	for {
		// Get user input
		prompt, err := cli.GetPrompt()
		if err == io.EOF {
			fmt.Println("\nGoodbye!")
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get prompt: %v", err)
		}

		if prompt == "" {
			continue
		}

		// Handle slash commands
		if cli.IsSlashCommand(prompt) {
			if cli.HandleSlashCommand(prompt, serverNames, toolNames, messages) {
				continue
			}
			cli.DisplayError(fmt.Errorf("unknown command: %s", prompt))
			continue
		}

		// Display user message
		cli.DisplayUserMessage(prompt)

		// Add user message to history
		messages = append(messages, schema.UserMessage(prompt))

		// Prune messages if needed
		if len(messages) > messageWindow {
			messages = messages[len(messages)-messageWindow:]
		}

		// Get agent response with controlled spinner that stops for tool call display
		var response *schema.Message
		var currentSpinner *ui.Spinner
		
		// Start initial spinner
		currentSpinner = ui.NewSpinner("Thinking...")
		currentSpinner.Start()
		
		response, err = mcpAgent.GenerateWithLoop(ctx, messages,
			// Tool call handler - called when a tool is about to be executed
			func(toolName, toolArgs string) {
				// Stop spinner before displaying tool call
				if currentSpinner != nil {
					currentSpinner.Stop()
					currentSpinner = nil
				}
				cli.DisplayToolCallMessage(toolName, toolArgs)
			},
			// Tool execution handler - called when tool execution starts/ends
			func(toolName string, isStarting bool) {
				if isStarting {
					// Start spinner for tool execution
					currentSpinner = ui.NewSpinner(fmt.Sprintf("Executing %s...", toolName))
					currentSpinner.Start()
				} else {
					// Stop spinner when tool execution completes
					if currentSpinner != nil {
						currentSpinner.Stop()
						currentSpinner = nil
					}
				}
			},
			// Tool result handler - called when a tool execution completes
			func(toolName, toolArgs, result string, isError bool) {
				cli.DisplayToolMessage(toolName, toolArgs, result, isError)
				// Start spinner again for next LLM call
				currentSpinner = ui.NewSpinner("Thinking...")
				currentSpinner.Start()
			},
			// Response handler - called when the LLM generates a response
			func(content string) {
				// Stop spinner when we get the final response
				if currentSpinner != nil {
					currentSpinner.Stop()
					currentSpinner = nil
				}
			},
			// Tool call content handler - called when content accompanies tool calls
			func(content string) {
				// Stop spinner before displaying content
				if currentSpinner != nil {
					currentSpinner.Stop()
					currentSpinner = nil
				}
				cli.DisplayAssistantMessageWithModel(content, modelName)
				// Start spinner again for tool calls
				currentSpinner = ui.NewSpinner("Thinking...")
				currentSpinner.Start()
			},
		)
		
		// Make sure spinner is stopped if still running
		if currentSpinner != nil {
			currentSpinner.Stop()
		}
		if err != nil {
			cli.DisplayError(fmt.Errorf("agent error: %v", err))
			continue
		}

		// Display assistant response with model name
		if err := cli.DisplayAssistantMessageWithModel(response.Content, modelName); err != nil {
			cli.DisplayError(fmt.Errorf("display error: %v", err))
		}

		// Add assistant response to history
		messages = append(messages, response)
	}
}

// ScriptConfig represents the YAML frontmatter in a script file
type ScriptConfig struct {
	MCPServers map[string]config.MCPServerConfig `yaml:"mcpServers"`
	Prompt     string                            `yaml:"prompt"`
}

// runScriptMode handles script mode execution
func runScriptMode(ctx context.Context) error {
	var scriptFile string
	
	// Determine script file from arguments
	// When called via shebang, the script file is the first non-flag argument
	// When called with --script flag, we need to find the script file in args
	args := os.Args[1:]
	
	// Filter out flags to find the script file
	for _, arg := range args {
		if arg == "--script" {
			// Skip the --script flag itself
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// Skip other flags
			continue
		}
		// This should be our script file
		scriptFile = arg
		break
	}
	
	if scriptFile == "" {
		return fmt.Errorf("script mode requires a script file argument")
	}
	
	// Parse the script file
	scriptConfig, prompt, err := parseScriptFile(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to parse script file: %v", err)
	}
	
	// Override the global configFile and promptFlag with script values
	originalConfigFile := configFile
	originalPromptFlag := promptFlag
	
	// Create config from script or load normal config
	var mcpConfig *config.Config
	if len(scriptConfig.MCPServers) > 0 {
		// Use servers from script
		mcpConfig = &config.Config{
			MCPServers: scriptConfig.MCPServers,
		}
	} else {
		// Fall back to normal config loading
		mcpConfig, err = config.LoadMCPConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load MCP config: %v", err)
		}
	}
	
	// Override the global config for normal mode
	scriptMCPConfig = mcpConfig
	
	// Set the prompt from script
	promptFlag = prompt
	
	// Restore original values after execution
	defer func() {
		configFile = originalConfigFile
		promptFlag = originalPromptFlag
		scriptMCPConfig = nil
	}()
	
	// Now run the normal execution path which will use our overridden config
	return runNormalMode(ctx)
}

// parseScriptFile parses a script file with YAML frontmatter and prompt
func parseScriptFile(filename string) (*ScriptConfig, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	
	// Skip shebang line if present
	if scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#!") {
			// If it's not a shebang, we need to process this line
			return parseScriptContent(line + "\n" + readRemainingLines(scanner))
		}
	}
	
	// Read the rest of the file
	content := readRemainingLines(scanner)
	return parseScriptContent(content)
}

// readRemainingLines reads all remaining lines from a scanner
func readRemainingLines(scanner *bufio.Scanner) string {
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}

// parseScriptContent parses the content to extract YAML frontmatter and prompt
func parseScriptContent(content string) (*ScriptConfig, string, error) {
	lines := strings.Split(content, "\n")
	
	// Find YAML frontmatter and prompt
	var yamlLines []string
	var promptLines []string
	var inPrompt bool
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "prompt:") {
			inPrompt = true
			// Extract the prompt value if it's on the same line
			if len(trimmed) > 7 {
				promptValue := strings.TrimSpace(trimmed[7:])
				if promptValue != "" {
					promptLines = append(promptLines, promptValue)
				}
			}
			continue
		}
		
		if inPrompt {
			// Continue collecting prompt lines (handle multi-line YAML strings)
			if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
				promptLines = append(promptLines, strings.TrimPrefix(strings.TrimPrefix(line, "  "), "\t"))
			} else if trimmed != "" && !strings.Contains(trimmed, ":") {
				promptLines = append(promptLines, line)
			} else if trimmed != "" {
				// New YAML key, stop collecting prompt
				inPrompt = false
				yamlLines = append(yamlLines, line)
			}
		} else {
			yamlLines = append(yamlLines, line)
		}
	}
	
	// Parse YAML
	yamlContent := strings.Join(yamlLines, "\n")
	var scriptConfig ScriptConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &scriptConfig); err != nil {
		return nil, "", fmt.Errorf("failed to parse YAML: %v", err)
	}
	
	// Join prompt lines
	prompt := strings.Join(promptLines, "\n")
	prompt = strings.TrimSpace(prompt)
	
	// If prompt wasn't found in YAML, use the scriptConfig.Prompt
	if prompt == "" {
		prompt = scriptConfig.Prompt
	}
	
	return &scriptConfig, prompt, nil
}