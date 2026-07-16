package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the full program configuration.
type Config struct {
	HashConfig HashConfig
	Force      bool
	Recursive  bool
	RegexMode  bool
	Quiet      bool
	Verbose    bool
	AutoYes    bool
	SearchDir  string
	NoWarn     bool // skip truncation-risk prompt if already confirmed
}

// pkgVerbose is set from cfg.Verbose so logVerbose can check it.
var pkgVerbose bool

func main() {
	cfg := parseFlags()

	// ── Truncation risk prompt ──────────────────────────────────
	if cfg.HashConfig.Truncate > 0 && cfg.HashConfig.Truncate <= 8 && !cfg.AutoYes {
		if !promptTruncationRisk(cfg.HashConfig) {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}
	fmt.Fprintf(os.Stderr, "\n")

	// ── Collect files ───────────────────────────────────────────
	files, err := collectFiles(flag.Args(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("No files to process.")
		return
	}

	if cfg.Verbose {
		fmt.Printf("Found %d file(s)\n", len(files))
	}

	pkgVerbose = cfg.Verbose

	// ── Process ─────────────────────────────────────────────────
	exitCode := processFiles(files, cfg)

	if !cfg.Quiet {
		fmt.Println("Done.")
	}
	os.Exit(exitCode)
}

// ---------------------------------------------------------------------------
// Flag parsing
// ---------------------------------------------------------------------------

func parseFlags() *Config {
	// Default config
	hashStr := flag.String("hash", "sha3-224", "Hash algorithm (e.g. sha3-224, SHA-256, sha3-224:16)")
	// Using env var or default
	force := flag.Bool("force", false, "Force-delete on collision even if sha3-512 differs")
	forceF := flag.Bool("f", false, "Alias for --force")
	recursive := flag.Bool("recursive", false, "Process subdirectories recursively")
	recursiveR := flag.Bool("r", false, "Alias for --recursive")
	regexMode := flag.Bool("regex", false, "Use extended regex for file matching (like grep -E)")
	regexE := flag.Bool("e", false, "Alias for --regex")
	quiet := flag.Bool("quiet", false, "Suppress non-essential output")
	quietQ := flag.Bool("q", false, "Alias for --quiet")
	verbose := flag.Bool("verbose", false, "Verbose output")
	verboseV := flag.Bool("v", false, "Alias for --verbose")
	autoYes := flag.Bool("yes", false, "Auto-confirm prompts (e.g. short truncation warning)")
	autoYesY := flag.Bool("y", false, "Alias for --yes")
	help := flag.Bool("help", false, "Show help")
	helpH := flag.Bool("h", false, "Alias for --help")

	flag.Usage = printUsage
	flag.Parse()

	// Show help if requested
	if *help || *helpH {
		printUsage()
		os.Exit(0)
	}

	// Merge aliases
	fForce := *force || *forceF
	fRecursive := *recursive || *recursiveR
	fRegex := *regexMode || *regexE
	fQuiet := *quiet || *quietQ
	fVerbose := *verbose || *verboseV
	fAutoYes := *autoYes || *autoYesY

	// Quiet and verbose are mutually exclusive
	if fQuiet && fVerbose {
		fmt.Fprintln(os.Stderr, "Warning: --quiet and --verbose are mutually exclusive; --verbose wins")
		fQuiet = false
	}

	// Parse hash config
	hashCfg, err := ParseHashConfig(*hashStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Determine search dir: positional args or "."
	searchDir := "."
	if flag.NArg() > 0 {
		// Try to find a common directory base for regex mode
		searchDir = inferSearchDir(flag.Args(), fRegex)
	}

	cfg := &Config{
		HashConfig: hashCfg,
		Force:      fForce,
		Recursive:  fRecursive,
		RegexMode:  fRegex,
		Quiet:      fQuiet,
		Verbose:    fVerbose,
		AutoYes:    fAutoYes,
		SearchDir:  searchDir,
	}

	if !fQuiet {
		fmt.Fprintf(os.Stderr, "🔐 Hash: %s", cfg.HashConfig.DisplayName())
		if fForce {
			fmt.Fprintf(os.Stderr, "  [--force]")
		}
		if fRecursive {
			fmt.Fprintf(os.Stderr, "  [recursive]")
		}
		if fRegex {
			fmt.Fprintf(os.Stderr, "  [regex]")
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	return cfg
}

// inferSearchDir tries to figure out the base directory from positional args.
// For regex mode, the search directory is typically the first existing
// directory in the arg list, or ".".
func inferSearchDir(args []string, regexMode bool) string {
	if !regexMode {
		// Glob mode: patterns are paths, so just use "."
		return "."
	}
	for _, a := range args {
		if info, err := os.Stat(a); err == nil && info.IsDir() {
			return a
		}
	}
	// Check if any arg is a path with a directory component
	for _, a := range args {
		dir := filepath.Dir(a)
		if dir != "." && dir != "/" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	return "."
}

// ---------------------------------------------------------------------------
// Prompt helpers
// ---------------------------------------------------------------------------

func promptTruncationRisk(cfg HashConfig) bool {
	bits := cfg.Truncate * 4 / 2 // hex chars → bits → 4 bits per char, /2 for birthday bound
	fmt.Fprintf(os.Stderr,
		"⚠  Truncated hash: %s\n"+
			"   Using only the first %d hex character(s) (~%d bits of collision resistance).\n"+
			"   This significantly increases the risk of accidental filename collisions.\n\n"+
			"Are you sure you want to continue? [y/N] ",
		cfg.DisplayName(), cfg.Truncate, bits)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes"
}

// ---------------------------------------------------------------------------
// Logging helpers
// ---------------------------------------------------------------------------

func logVerbose(format string, args ...interface{}) {
	if pkgVerbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: fileRenamer [options] [pattern ...]

Rename files to their cryptographic hash, with deduplication.

Hash algorithms (case-insensitive, SHA-2 optional dash):
  sha3-224         (default)     SHA-224 / sha224
  sha3-256                        SHA-256 / sha256
  sha3-384                        SHA-384 / sha384
  sha3-512                        SHA-512 / sha512
                                  SHA-512/224
                                  SHA-512/256

  Truncation: algo:N  —  e.g. sha3-224:16 (first 16 hex chars)
  ⚠  N ≤ 8 triggers a collision-risk confirmation prompt.

Options:
  -hash <algo>      Hash algorithm (default: "sha3-224")
  -force, -f        Force-delete on collision even if sha3-512 differs
  -recursive, -r    Process subdirectories recursively
  -regex, -e        Use extended regex for file matching (like grep -E)
  -quiet, -q        Suppress non-essential output
  -verbose, -v      Verbose output
  -yes, -y          Auto-confirm prompts (e.g. truncation warnings)
  -help, -h         Show this help

Patterns:
  Default (no args):   process all files in ./
  Glob:                fileRenamer ./*.jpg

  Regex (grep-style):
    First positional arg = regex pattern
    Rest = directories to search (default: ./)
    Examples:
      fileRenamer -e '\.txt$'
      fileRenamer -e '\.go$' ./src ./tests

Collision resolution:
  1. Same primary hash + same sha3-512  →  duplicate → delete newer file
  2. Same primary hash + diff sha3-512  →  WARNING (real collision), file kept in place
  3. --force                           →  always delete newer, skip sha3-512 check

Examples:
  fileRenamer                                  Process ./ with sha3-224
  fileRenamer -hash SHA-256                    Use SHA-256
  fileRenamer -hash sha3-224:16                Use first 16 hex chars of sha3-224
  fileRenamer --force ./*.jpg                  Deduplicate JPGs with force
  fileRenamer -r -e '\.go$'                   Recursively rename all .go files
  fileRenamer -hash sha256 *.txt               Use SHA-256 on .txt files
`,
)
}
