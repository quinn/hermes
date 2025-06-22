package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// FontsYAML represents the schema of fonts.yaml
// Example:
// fonts:
//   - family: "Roboto"
//     variants: ["regular", "700italic"]
//
// dir: "./webfonts"
// stylesheet: "./fonts.css"
type FontsYAML struct {
	Fonts      []FontEntry `yaml:"fonts"`
	Dir        string      `yaml:"dir"`
	Stylesheet string      `yaml:"stylesheet"`
}

type FontEntry struct {
	Family   string   `yaml:"family"`
	Variants []string `yaml:"variants"`
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install multiple fonts and variants from a fonts.yaml file",
	Long:  `Reads fonts.yaml and installs all specified fonts/variants, saving files and stylesheet as specified in the YAML.`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose := true // Always verbose for now
		configPath := "fonts.yaml"
		if len(args) > 0 {
			configPath = args[0]
		}
		if verbose {
			fmt.Printf("Reading font configuration from %s...\n", configPath)
		}
		cfg, err := readFontsYAML(configPath)
		if err != nil {
			fmt.Printf("Error reading YAML: %v\n", err)
			os.Exit(1)
		}
		if verbose {
			fmt.Printf("Installing fonts to directory: %s\n", cfg.Dir)
		}
		if cfg.Dir == "" {
			fmt.Println("Error: `dir` not specified in YAML")
			os.Exit(1)
		}
		if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
			fmt.Printf("Failed to create directory %s: %v\n", cfg.Dir, err)
			os.Exit(1)
		}
		if cfg.Stylesheet == "" {
			fmt.Println("Error: `stylesheet` not specified in YAML")
			os.Exit(1)
		}
		if err := os.MkdirAll(filepath.Dir(cfg.Stylesheet), 0755); err != nil {
			fmt.Printf("Failed to create directory %s: %v\n", cfg.Stylesheet, err)
			os.Exit(1)
		}
		// Track all font files that should exist after install
		wantedFiles := map[string]struct{}{}
		cssRules := []string{}
		if verbose && len(cfg.Fonts) == 0 {
			fmt.Printf("No fonts specified in YAML\n")
			os.Exit(1)
		}
		for _, entry := range cfg.Fonts {
			parsedFamily := parseFontFamily(entry.Family)
			fontResponse := getFontUrl(parsedFamily)
			if len(fontResponse.Items) < 1 {
				fmt.Printf("Warning: No font found for %s\n", entry.Family)
				continue
			}
			item := fontResponse.Items[0]
			files := item.Files
			for _, variant := range entry.Variants {
				url, ok := files[variant]
				if !ok {
					fmt.Printf("Variant %s not found for %s\n", variant, entry.Family)
					continue
				}
				fileName := item.Family + "_" + variant + ".woff2"
				filePath := filepath.Join(cfg.Dir, fileName)
				if verbose {
					fmt.Printf("Downloading %s (%s) -> %s\n", entry.Family, variant, filePath)
				}
				if err := downloadToFile(url, filePath); err != nil {
					fmt.Printf("Failed to download %s: %v\n", fileName, err)
					continue
				}
				wantedFiles[fileName] = struct{}{}
				cssRules = append(cssRules, genCSS(item.Family, variant, fileName))
			}
		}
		// Remove any font files in dir not referenced in wantedFiles
		removeUnreferencedFiles(cfg.Dir, wantedFiles, verbose)
		// Write CSS file
		if verbose {
			fmt.Printf("Writing CSS to %s\n", cfg.Stylesheet)
		}
		if err := writeCSS(cfg.Stylesheet, cssRules); err != nil {
			fmt.Printf("Failed to write CSS: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\nInstall complete!")
	},
}

func readFontsYAML(path string) (*FontsYAML, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg FontsYAML
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func downloadToFile(url, filePath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func removeUnreferencedFiles(dir string, wanted map[string]struct{}, verbose bool) {
	d, err := os.Open(dir)
	if err != nil {
		fmt.Printf("Failed to open directory for cleanup: %v\n", err)
		return
	}
	defer d.Close()
	files, err := d.Readdirnames(-1)
	if err != nil {
		fmt.Printf("Failed to list directory: %v\n", err)
		return
	}
	for _, f := range files {
		if !strings.HasSuffix(f, ".woff2") {
			continue
		}
		if _, ok := wanted[f]; !ok {
			fullPath := filepath.Join(dir, f)
			if verbose {
				fmt.Printf("Removing unreferenced font file: %s\n", fullPath)
			}
			os.Remove(fullPath)
		}
	}
}

func writeCSS(path string, rules []string) error {
	css := strings.Join(rules, "\n\n")
	return os.WriteFile(path, []byte(css), 0644)
}

func genCSS(family, variant, fileName string) string {
	style := "normal"
	weight := "400"
	if variant == "italic" {
		style = "italic"
	} else if strings.HasSuffix(variant, "italic") {
		style = "italic"
		weight = strings.TrimSuffix(variant, "italic")
	} else if variant != "regular" {
		weight = variant
	}
	return fmt.Sprintf(`@font-face {
  font-family: '%s';
  font-style: %s;
  font-weight: %s;
  src: url('%s') format('woff2');
}`, family, style, weight, fileName)
}

func init() {
	rootCmd.AddCommand(installCmd)
}
