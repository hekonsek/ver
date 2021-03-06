package vrs

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const VrsConfigFileName = "vrs.yml"

type VrsConfig struct {
	Version  string
	Sync     *Sync      `yaml:",omitempty"`
	Profiles []*Profile `yaml:",omitempty"`
}

type Sync struct {
	Files []SyncFile
}

type SyncFile struct {
	Name    string
	Pattern string
}

type Profile struct {
	Name string
	Sync *Sync
}

var NoVersioonFileFound = errors.New("no vrs file found")

func ParseVersioonConfig(basePath string) (*VrsConfig, error) {
	versioonConfigPath := path.Join(basePath, VrsConfigFileName)
	if _, err := os.Stat(versioonConfigPath); err != nil {
		if os.IsNotExist(err) {
			return nil, NoVersioonFileFound
		}
	}

	yml, err := os.ReadFile(versioonConfigPath)
	if err != nil {
		return nil, err
	}

	config := &VrsConfig{}
	err = yaml.Unmarshal(yml, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (config *VrsConfig) Write(basePath string) error {
	yml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	err = os.WriteFile(path.Join(basePath, VrsConfigFileName), yml, 0600)
	if err != nil {
		return err
	}
	return nil
}

func (config *VrsConfig) WriteAndCommit(baseDir string, commit bool, push bool, commitMessage string) error {
	err := config.Write(baseDir)
	if err != nil {
		return err
	}

	if commit {
		cmd := exec.Command("git", "add", VrsConfigFileName)
		cmd.Dir = baseDir
		err = cmd.Run()
		if err != nil {
			return err
		}

		// #nosec - Git commit message is safe to set via variable.
		cmd = exec.Command("git", "commit", "-m", commitMessage)
		cmd.Dir = baseDir
		err = cmd.Run()
		if err != nil {
			return err
		}

		cmd = exec.Command("git", "tag", "v"+config.Version)
		cmd.Dir = baseDir
		err = cmd.Run()
		if err != nil {
			return err
		}

		if push {
			cmd = exec.Command("git", "push")
			cmd.Dir = baseDir
			err = cmd.Run()
			if err != nil {
				return err
			}

			cmd = exec.Command("git", "push", "--tags")
			cmd.Dir = baseDir
			err = cmd.Run()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type InitOptions struct {
	Basedir   string
	GitCommit bool
	GitPush   bool
}

func NewDefaultInitOptions() (*InitOptions, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &InitOptions{
		Basedir:   wd,
		GitCommit: true,
		GitPush:   true,
	}, nil
}

func Init(options *InitOptions) error {
	if options == nil {
		o, err := NewDefaultInitOptions()
		if err != nil {
			return err
		}
		options = o
	}
	err := (&VrsConfig{Version: "0.0.0"}).WriteAndCommit(options.Basedir, options.GitCommit, options.GitPush, "Initialized versioon file.")
	if err != nil {
		return err
	}
	return nil
}

type BumpOptions struct {
	Basedir        string
	GitCommit      bool
	GitPush        bool
	ActiveProfiles []string
}

func NewDefaultBumpOptions() (*BumpOptions, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &BumpOptions{
		Basedir:   wd,
		GitCommit: true,
		GitPush:   true,
	}, nil
}

func Bump(options *BumpOptions) error {
	if options == nil {
		o, err := NewDefaultBumpOptions()
		if err != nil {
			return err
		}
		options = o
	}

	config, err := ParseVersioonConfig(options.Basedir)
	if err != nil {
		return err
	}

	oldVersion := config.Version
	versionParts := strings.Split(oldVersion, ".")
	minorVersion, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return err
	}
	config.Version = fmt.Sprintf("%s.%d.%s", versionParts[0], minorVersion+1, versionParts[2])
	err = config.WriteAndCommit(options.Basedir, options.GitCommit, options.GitPush, "Version bump.")
	if err != nil {
		return err
	}

	if config.Sync != nil {
		for _, file := range config.Sync.Files {
			if file.Pattern == "" {
				err = bumpInFile(options.Basedir, options.GitCommit, file.Name, oldVersion, "", config.Version)
			} else {
				err = bumpInFile(options.Basedir, options.GitCommit, file.Name, "", file.Pattern, config.Version)
			}
			if err != nil {
				return err
			}
		}
	}

	for _, profile := range config.Profiles {
		for _, activeProfile := range options.ActiveProfiles {
			if activeProfile == profile.Name {
				if profile.Sync != nil {
					for _, file := range profile.Sync.Files {
						if file.Pattern == "" {
							err = bumpInFile(options.Basedir, options.GitCommit, file.Name, oldVersion, "", config.Version)
						} else {
							err = bumpInFile(options.Basedir, options.GitCommit, file.Name, "", file.Pattern, config.Version)
						}
						if err != nil {
							return err
						}
					}
				}
				break
			}
		}
	}

	return nil
}

func bumpInFile(baseDir string, gitCommit bool, file string, oldVersion string, oldExpression string, newVersion string) error {
	filePath := path.Join(baseDir, file)
	originalBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	bumpedFile := ""
	if oldVersion != "" {
		bumpedFile = strings.ReplaceAll(string(originalBytes), oldVersion, newVersion)
	} else {
		r, err := regexp.Compile(oldExpression)
		if err != nil {
			return err
		}
		bumpedFile = r.ReplaceAllString(string(originalBytes), newVersion)
	}

	err = os.WriteFile(filePath, []byte(bumpedFile), 0600)
	if err != nil {
		return err
	}

	if gitCommit {
		cmd := exec.Command("git", "add", file)
		cmd.Dir = baseDir
		err = cmd.Run()
		if err != nil {
			return err
		}

		cmd = exec.Command("git", "commit", "-m", "Bumped version.")
		cmd.Dir = baseDir
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

type ReadCurrentOptions struct {
	Basedir   string
	GitCommit bool
	GitPush   bool
}

func NewDefaultReadCurrentOptions() (*ReadCurrentOptions, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &ReadCurrentOptions{
		Basedir:   wd,
		GitCommit: true,
		GitPush:   true,
	}, nil
}

func ReadCurrentVersion(options *ReadCurrentOptions) (string, error) {
	if options == nil {
		o, err := NewDefaultReadCurrentOptions()
		if err != nil {
			return "", err
		}
		options = o
	}

	config, err := ParseVersioonConfig(options.Basedir)
	if err != nil {
		return "", err
	}
	return config.Version, nil
}
