package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/djherbis/times"
)

// ---------------------------------------------------------------------------
// Collision resolution results
// ---------------------------------------------------------------------------

type CollisionAction int

const (
	ActionRenamed     CollisionAction = iota // file was renamed OK
	ActionDeleted                            // file was deleted as duplicate
	ActionSkipped                            // file was skipped (real collision warning)
	ActionKeepOriginal                       // file kept original name (collision without --force)
	ActionError                              // error occurred
)

// ProcessedEntry tracks a file we've already handled.
type ProcessedEntry struct {
	Path        string
	PrimaryHash string
	Sha3512     string
	BirthTime   time.Time
	OrderIndex  int
}

// ---------------------------------------------------------------------------
// Birth time (cross-platform)
// ---------------------------------------------------------------------------

func getBirthTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	t := times.Get(info)
	if t.HasBirthTime() {
		return t.BirthTime(), nil
	}
	// Fallback to ModTime
	return info.ModTime(), nil
}

// ---------------------------------------------------------------------------
// File matching (glob / regex / literal)
// ---------------------------------------------------------------------------

func collectFiles(patterns []string, cfg *Config) ([]string, error) {
	// Regex mode: first arg = pattern, rest = search dirs (default: ".")
	if cfg.RegexMode {
		if len(patterns) == 0 {
			return nil, fmt.Errorf("regex mode requires a pattern (e.g. -e '\\.txt$')")
		}
		regexPat := patterns[0]
		searchDirs := patterns[1:]
		if len(searchDirs) == 0 {
			searchDirs = []string{cfg.SearchDir}
		}

		seen := map[string]bool{}
		var files []string
		for _, dir := range searchDirs {
			matched, err := matchRegex(regexPat, dir, cfg.Recursive)
			if err != nil {
				return nil, fmt.Errorf("regex %q in %s: %w", regexPat, dir, err)
			}
			for _, m := range matched {
				abs, _ := filepath.Abs(m)
				if !seen[abs] {
					seen[abs] = true
					files = append(files, abs)
				}
			}
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("regex %q: no matches", regexPat)
		}
		return files, nil
	}

	// No args: scan search directory
	if len(patterns) == 0 {
		return listDirFiles(cfg.SearchDir, cfg.Recursive)
	}

	// Glob mode: each arg is a glob pattern or directory path
	seen := map[string]bool{}
	var files []string
	for _, pat := range patterns {
		// Check if the arg is an existing directory — if so, list its files
		if info, err := os.Stat(pat); err == nil && info.IsDir() {
			dirFiles, err := listDirFiles(pat, cfg.Recursive)
			if err != nil {
				return nil, fmt.Errorf("reading directory %q: %w", pat, err)
			}
			for _, m := range dirFiles {
				abs, _ := filepath.Abs(m)
				if !seen[abs] {
					seen[abs] = true
					files = append(files, abs)
				}
			}
			continue
		}

		matched, err := matchGlob(pat)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w (try --regex/-e for regex mode)", pat, err)
		}
		for _, m := range matched {
			abs, _ := filepath.Abs(m)
			if !seen[abs] {
				seen[abs] = true
				files = append(files, abs)
			}
		}
	}

	return files, nil
}

func listDirFiles(dir string, recursive bool) ([]string, error) {
	dir = filepath.Clean(dir)
	var files []string

	if recursive {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == dir {
				return nil
			}
			if !d.IsDir() && d.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
		return files, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && e.Type().IsRegular() {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

func matchGlob(pattern string) ([]string, error) {
	matched, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("no matches")
	}
	// Filter out directories
	var files []string
	for _, m := range matched {
		info, err := os.Stat(m)
		if err == nil && !info.IsDir() && info.Mode().IsRegular() {
			files = append(files, m)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no matching files")
	}
	return files, nil
}

func matchRegex(pattern string, searchDir string, recursive bool) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var allFiles []string
	if recursive {
		err = filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && d.Type().IsRegular() {
				allFiles = append(allFiles, path)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(searchDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && e.Type().IsRegular() {
				allFiles = append(allFiles, filepath.Join(searchDir, e.Name()))
			}
		}
	}
	if err != nil {
		return nil, err
	}

	var matched []string
	for _, f := range allFiles {
		// Match against the relative path from searchDir
		rel, _ := filepath.Rel(searchDir, f)
		if re.MatchString(rel) || re.MatchString(f) {
			matched = append(matched, f)
		}
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("regex %q: no matches in %s", pattern, searchDir)
	}
	return matched, nil
}

// ---------------------------------------------------------------------------
// Main processing
// ---------------------------------------------------------------------------

// FileInfo holds pre-computed info about a file before processing.
type FileInfo struct {
	Path      string
	BirthTime time.Time
}

func processFiles(filePaths []string, cfg *Config) int {
	// Exclude the current running executable from processing
	filePaths = filterSelfExe(filePaths)

	// Gather birth times and sort
	infos := make([]FileInfo, 0, len(filePaths))
	for _, p := range filePaths {
		bt, err := getBirthTime(p)
		if err != nil {
			logVerbose("Cannot get birth time for %s: %v, using mtime", filepath.Base(p), err)
			info, err2 := os.Stat(p)
			if err2 != nil {
				logError("Cannot stat %s: %v", p, err2)
				continue
			}
			bt = info.ModTime()
		}
		infos = append(infos, FileInfo{Path: p, BirthTime: bt})
	}

	// Sort oldest-first; ties broken by path
	sort.Slice(infos, func(i, j int) bool {
		if !infos[i].BirthTime.Equal(infos[j].BirthTime) {
			return infos[i].BirthTime.Before(infos[j].BirthTime)
		}
		return infos[i].Path < infos[j].Path
	})

	processed := map[string][]*ProcessedEntry{} // key = primary hash (uppercase)
	exitCode := 0

	for i, fi := range infos {
		// Does the file still exist? (might have been deleted as duplicate)
		if _, err := os.Stat(fi.Path); os.IsNotExist(err) {
			logVerbose("Skipping %s: already removed as duplicate", filepath.Base(fi.Path))
			continue
		}

		baseName := filepath.Base(fi.Path)
		ext := strings.ToLower(filepath.Ext(fi.Path))
		nameStem := strings.TrimSuffix(baseName, filepath.Ext(baseName))

		// Log progress
		logVerbose("[%d/%d] %s", i+1, len(infos), baseName)

		// --- Compute primary hash ---
		primaryHash, err := cfg.HashConfig.HashFile(fi.Path)
		if err != nil {
			logError("Hash failed: %s: %v", baseName, err)
			exitCode = 1
			continue
		}

		// Already named correctly? skip.
		if strings.EqualFold(nameStem, primaryHash) {
			logVerbose("  Already named correctly, skipping")
			if !cfg.Quiet {
				fmt.Printf("✓ %s (already hashed)\n", baseName)
			}
			continue
		}

		// --- Compute sha3-512 for collision verification ---
		sha3512Hash, err := Sha3512HashFile(fi.Path)
		if err != nil {
			logError("sha3-512 failed: %s: %v", baseName, err)
			exitCode = 1
			continue
		}

		// --- Collision check ---
		if existingList, collision := processed[primaryHash]; collision {
			action := resolveCollision(fi, primaryHash, sha3512Hash, existingList, cfg, i)
			switch action {
			case ActionDeleted:
				// File was removed; do not add to processed
				continue
			case ActionKeepOriginal:
				// Real collision, file kept in place — still track it
				processed[primaryHash] = append(processed[primaryHash], &ProcessedEntry{
					Path:        fi.Path,
					PrimaryHash: primaryHash,
					Sha3512:     sha3512Hash,
					BirthTime:   fi.BirthTime,
					OrderIndex:  i,
				})
				continue
			default:
				continue
			}
		}

		// --- No collision — rename ---
		newName := primaryHash + ext
		dir := filepath.Dir(fi.Path)
		newPath := filepath.Join(dir, newName)

		// Does the target path already exist? (collision with pre-existing file)
		if _, err := os.Stat(newPath); err == nil {
			action := resolveTargetExists(fi, primaryHash, sha3512Hash, newPath, cfg, i)
			switch action {
			case ActionDeleted:
				continue
			case ActionKeepOriginal:
				processed[primaryHash] = append(processed[primaryHash], &ProcessedEntry{
					Path:        fi.Path,
					PrimaryHash: primaryHash,
					Sha3512:     sha3512Hash,
					BirthTime:   fi.BirthTime,
					OrderIndex:  i,
				})
				continue
			default:
				continue
			}
		}

		// Safe to rename
		if err := os.Rename(fi.Path, newPath); err != nil {
			logError("Rename failed: %s → %s: %v", baseName, newName, err)
			exitCode = 1
			continue
		}

		if cfg.Verbose {
			fmt.Printf("Renamed: %s → %s\n", baseName, newName)
		} else if !cfg.Quiet {
			fmt.Printf("Renamed: %s → %s\n", baseName, newName)
		}

		processed[primaryHash] = append(processed[primaryHash], &ProcessedEntry{
			Path:        newPath,
			PrimaryHash: primaryHash,
			Sha3512:     sha3512Hash,
			BirthTime:   fi.BirthTime,
			OrderIndex:  i,
		})
	}

	return exitCode
}

// resolveCollision handles the case where a newly-processed file has the same
// primary hash as one or more previously-processed files.
//
// Returns ActionDeleted (file removed), ActionKeepOriginal (real collision),
// or ActionSkipped.
func resolveCollision(
	fi FileInfo, primaryHash, sha3512Hash string,
	existing []*ProcessedEntry, cfg *Config, orderIdx int,
) CollisionAction {
	baseName := filepath.Base(fi.Path)

	// Look for a match in sha3-512
	matchFound := false
	var matchedEntry *ProcessedEntry
	for _, e := range existing {
		if e.Sha3512 == sha3512Hash {
			matchFound = true
			matchedEntry = e
			break
		}
	}

	if matchFound {
		// True duplicate: same content. Delete the newer file (current).
		if err := os.Remove(fi.Path); err != nil {
			logError("Failed to delete duplicate %s: %v", baseName, err)
			return ActionError
		}
		if cfg.Verbose {
			fmt.Printf("Deleted duplicate: %s (same content as %s)\n",
				baseName, filepath.Base(matchedEntry.Path))
		} else if !cfg.Quiet {
			fmt.Printf("Deleted duplicate: %s\n", baseName)
		}
		return ActionDeleted
	}

	// Primary hash matches but sha3-512 differs: real hash collision (extremely rare)
	if cfg.Force {
		// --force: delete the newer file anyway
		if err := os.Remove(fi.Path); err != nil {
			logError("Failed to delete %s (--force): %v", baseName, err)
			return ActionError
		}
		fmt.Printf("Deleted (--force): %s\n", baseName)
		return ActionDeleted
	}

	// Warn and keep the file at its original name
	fmt.Fprintf(os.Stderr, "\n⚠ WARNING: Real hash collision detected on %s!\n", cfg.HashConfig.DisplayName())
	fmt.Fprintf(os.Stderr, "  File 1 (already renamed): %s\n", filepath.Base(existing[0].Path))
	fmt.Fprintf(os.Stderr, "    %s = %s\n", cfg.HashConfig.DisplayName(), primaryHash)
	fmt.Fprintf(os.Stderr, "    sha3-512 = %s\n", existing[0].Sha3512)
	fmt.Fprintf(os.Stderr, "  File 2: %s\n", baseName)
	fmt.Fprintf(os.Stderr, "    %s = %s\n", cfg.HashConfig.DisplayName(), primaryHash)
	fmt.Fprintf(os.Stderr, "    sha3-512 = %s\n", sha3512Hash)
	fmt.Fprintf(os.Stderr, "  → File 2 will NOT be renamed (kept in place)\n\n")
	return ActionKeepOriginal
}

// resolveTargetExists handles the case where the target rename path already
// exists as a file not in our processed set.
func resolveTargetExists(
	fi FileInfo, primaryHash, sha3512Hash, targetPath string,
	cfg *Config, orderIdx int,
) CollisionAction {
	baseName := filepath.Base(fi.Path)
	targetBase := filepath.Base(targetPath)

	// Compute sha3-512 of the existing target
	targetSha3512, err := Sha3512HashFile(targetPath)
	if err != nil {
		logError("Cannot verify existing target %s: %v", targetBase, err)
		return ActionError
	}

	if targetSha3512 == sha3512Hash {
		// Same content — delete the current file (newer)
		if err := os.Remove(fi.Path); err != nil {
			logError("Failed to delete duplicate %s: %v", baseName, err)
			return ActionError
		}
		if cfg.Verbose {
			fmt.Printf("Deleted duplicate: %s (same content as %s)\n", baseName, targetBase)
		} else if !cfg.Quiet {
			fmt.Printf("Deleted duplicate: %s\n", baseName)
		}
		return ActionDeleted
	}

	// Different content
	if cfg.Force {
		if err := os.Remove(fi.Path); err != nil {
			logError("Failed to delete %s (--force): %v", baseName, err)
			return ActionError
		}
		fmt.Printf("Deleted (--force): %s\n", baseName)
		return ActionDeleted
	}

	// Warn and keep in place
	fmt.Fprintf(os.Stderr, "\n⚠ WARNING: Target file already exists with different content!\n")
	fmt.Fprintf(os.Stderr, "  Target: %s (%s = %s)\n",
		targetBase, cfg.HashConfig.DisplayName(), primaryHash)
	fmt.Fprintf(os.Stderr, "  Source: %s\n", baseName)
	fmt.Fprintf(os.Stderr, "  → Source will NOT be renamed (kept in place)\n\n")
	return ActionKeepOriginal
}

// filterSelfExe removes the current running executable from the file list.
func filterSelfExe(paths []string) []string {
	exe, err := os.Executable()
	if err != nil {
		return paths
	}
	exeAbs, err := filepath.Abs(exe)
	if err != nil {
		return paths
	}

	out := make([]string, 0, len(paths))
	for _, p := range paths {
		pAbs, err := filepath.Abs(p)
		if err != nil || pAbs != exeAbs {
			out = append(out, p)
		}
	}
	return out
}
