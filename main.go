package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	_ "embed"

	"github.com/manifoldco/promptui"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ADRDirectory string `yaml:"adr_directory,omitempty"`
	ADRTemplate  string `yaml:"adr_template,omitempty"`
	TOCTemplate  string `yaml:"toc_template,omitempty"`
	DateFormat   string `yaml:"date_format,omitempty"`
}

func NewConfig() Config {
	config := Config{}
	config.ADRDirectory = "./docs/adr"
	config.ADRTemplate = ""
	config.TOCTemplate = ""
	config.DateFormat = "2006/01/02"
	return config
}

type ADR struct {
	Number int    `yaml:"number"`
	Name   string `yaml:"name"`
	Date   string `yaml:"date"`
	Status string `yaml:"status"`
}

type TOCEntry struct {
	Number int
	Name   string
	Link   string
}

//go:embed templates/adr.md
var adrTemplate string

//go:embed templates/first.md
var firstTemplate string

//go:embed templates/toc.md
var tocTemplate string

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	var config Config

	app := &cli.App{
		Name:  "adr",
		Usage: "Create architecture decision records from templates",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   ".adr.yaml",
				Usage:   "custom config file",
			},
		},
		Commands: []*cli.Command{
			{
				Name:        "version",
				Aliases:     []string{"v"},
				Usage:       "",
				Description: "Version information for adr",
				Action: func(c *cli.Context) error {
					fmt.Printf("Version: %s\nCommit: %s\nBuilt At: %s\nBuilt By: %s\n", version, commit, date, builtBy)
					return nil
				},
			},
			{
				Name:        "init",
				Aliases:     []string{"i"},
				Usage:       "",
				Description: "",
				Action: func(c *cli.Context) error {
					_, err := os.Stat(c.String("config"))
					if os.IsNotExist(err) {
						fmt.Println("creating config at .adr.yaml")

						newConfig, err := yaml.Marshal(NewConfig())
						if err != nil {
							log.Fatalln(err)
						}

						err = ioutil.WriteFile(c.String("config"), newConfig, 0644)
						if err != nil {
							log.Fatal(err)
						}

						config = readConfig(c.String("config"))

						fullPath, err := filepath.Abs(config.ADRDirectory)
						if err != nil {
							log.Fatalln(err)
						}

						if _, err := os.Stat(fullPath); os.IsNotExist(err) {
							err = os.MkdirAll(fullPath, os.ModePerm)
							if err != nil {
								log.Fatalln(err)
							}
						}

						adr := ADR{
							Number: 1,
							Name:   "Record architecture decisions",
							Status: "Accepted",
							Date:   time.Now().Format(config.DateFormat),
						}

						templateString := readTemplate("", firstTemplate)
						err = createADR(adr, templateString, fullPath)
						if err != nil {
							log.Fatal(err)
						}
					} else {
						log.Fatal("config already exists, project initialised")
					}

					return nil
				},
			}, {
				Name:        "new",
				Aliases:     []string{"n"},
				Description: "Create a new ADR",
				Before: func(c *cli.Context) error {
					config = readConfig(c.String("config"))
					return nil
				},
				Action: func(c *cli.Context) error {
					if _, err := os.Stat(".adr.yaml"); os.IsNotExist(err) {
						fmt.Println(".adr.yaml missing, run init first")
						os.Exit(1)
					}

					validate := func(input string) error {
						if len(input) >= 64 {
							return errors.New("Title must be shorter than 64 characters")
						}

						if len(input) <= 3 {
							return errors.New("Title must be longer than 3 characters")
						}

						return nil
					}

					namePrompt := promptui.Prompt{
						Label:    "Name",
						Validate: validate,
					}

					statusPrompt := promptui.Select{
						Label: "Status",
						Items: []string{"Accepted", "Proposed", "Rejected", "Superseeded"},
					}

					name, err := namePrompt.Run()
					if err != nil {
						log.Fatalln(err)
					}

					_, status, err := statusPrompt.Run()
					if err != nil {
						log.Fatalln(err)
					}

					fullPath, err := filepath.Abs(config.ADRDirectory)
					if err != nil {
						log.Fatalln(err)
					}

					adr := ADR{
						Name:   name,
						Status: status,
						Number: getNextNumber(fullPath),
						Date:   time.Now().Format(config.DateFormat),
					}

					templateString := readTemplate(config.ADRTemplate, adrTemplate)
					err = createADR(adr, templateString, fullPath)
					if err != nil {
						log.Fatal(err)
					}

					return nil
				},
			}, {
				Name:        "toc",
				Usage:       "",
				Description: "",
				Before: func(c *cli.Context) error {
					config = readConfig(c.String("config"))
					return nil
				},
				Action: func(c *cli.Context) error {
					fullPath, err := filepath.Abs(config.ADRDirectory)
					if err != nil {
						log.Fatalln(err)
					}

					files, err := ioutil.ReadDir(fullPath)
					if err != nil {
						log.Fatal(err)
					}

					var tocEntry []TOCEntry
					for _, file := range files {
						if strings.HasSuffix(file.Name(), ".yaml") {
							adrYaml, err := ioutil.ReadFile(filepath.Join(fullPath, file.Name()))
							if err != nil {
								log.Fatal(err)
							}

							var adr ADR
							err = yaml.Unmarshal(adrYaml, &adr)
							if err != nil {
								log.Fatal(err)
							}

							tocEntry = append(tocEntry, TOCEntry{
								Number: adr.Number,
								Name:   adr.Name,
								Link:   getADRFileName(adr) + ".md",
							})
						}
					}

					t, err := template.New("toc").Parse(readTemplate(config.TOCTemplate, tocTemplate))
					if err != nil {
						log.Fatalln(err)
					}

					f, err := os.Create(filepath.Join(fullPath, "README.md"))
					if err != nil {
						return err
					}

					err = t.Execute(f, tocEntry)
					if err != nil {
						log.Fatalln(err)
					}

					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func createADR(adr ADR, adrTemplate string, fullPath string) error {
	t, err := template.New("adr").Parse(adrTemplate)
	if err != nil {
		return err
	}

	adrFileName := getADRFileName(adr)
	adrFullPath := filepath.Join(fullPath, adrFileName)
	f, err := os.Create(adrFullPath + ".md")
	if err != nil {
		return err
	}

	err = t.Execute(f, adr)
	if err != nil {
		return err
	}

	adrYaml, err := yaml.Marshal(adr)
	if err != nil {
		log.Fatalln(err)
	}

	return ioutil.WriteFile(adrFullPath+".yaml", adrYaml, 0644)
}

func getADRFileName(adr ADR) string {
	re, err := regexp.Compile(`[^\w]`)
	if err != nil {
		log.Fatal(err)
	}
	adrName := re.ReplaceAllString(adr.Name, " ")

	return fmt.Sprintf("%04d", adr.Number) + "-" + strings.ToLower(strings.Join(strings.Split(strings.Trim(adrName, "\n \t"), " "), "-"))
}

func readConfig(configLocation string) Config {
	config := NewConfig()

	data, err := ioutil.ReadFile(configLocation)
	if err != nil {
		fmt.Println("no config found, using defaults")
		return config
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalln(err)
	}

	return config
}

func readTemplate(templateLocation string, defaultTemplate string) string {
	if templateLocation == "" {
		return defaultTemplate
	}

	data, err := ioutil.ReadFile(templateLocation)
	if err != nil {
		return defaultTemplate
	}

	return string(data)
}

func getNextNumber(directory string) int {
	files, err := ioutil.ReadDir(directory)
	if err != nil {
		log.Fatal(err)
	}

	nextNumber := 0

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".md") {
			adrNumber := getNumberFromADR(file.Name())

			if adrNumber > nextNumber {
				nextNumber = adrNumber
			}
		}
	}

	return nextNumber + 1
}

func getNumberFromADR(fileName string) int {
	parts := strings.SplitN(fileName, "-", 2)
	adrNumber, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}

	return adrNumber
}
