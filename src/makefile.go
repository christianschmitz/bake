package main

import (
  "errors"
  "fmt"
  "io/ioutil"
  "os"
  "os/exec"
  "path/filepath"
  "strings"
)

const (
  MAKEFILE = "Makefile"
)

func FindMakefileDir() (string, error) {
  curDir, err := os.Getwd()
  if err != nil {
    return "", err
  }

  found := false

  for len(curDir) > 1 {
    fname := filepath.Join(curDir, MAKEFILE)
    stat, err := os.Stat(fname)
    if err == nil {
      if stat.IsDir() {
        return "", errors.New(fname + " is directory")
      }

      found = true
      break
    }

    // move up one
    curDir = filepath.Dir(curDir)
  }

  if !found {
    return "", errors.New(MAKEFILE + " not found")
  } 

  return curDir, nil
}

func MakefileExists(dir string) (bool, error) {
  fname := filepath.Join(dir, MAKEFILE)

  stat, err := os.Stat(fname)
  if err != nil {
    if os.IsNotExist(err) {
      return false, nil
    } else {
      return false, err
    }
  }

  if stat.IsDir() {
    return false, errors.New(fname + " is directory")
  }

  return true, nil
}

func TargetExists(dir string, target string) (bool, error) {
  cmdName := "make"
  cmdArgs := []string{"-C", dir, "-n", target}

  cmd := exec.Command(cmdName, cmdArgs...)

  _, err_ := cmd.Output()
  if err_ == nil {
    return true, nil
  } else if err, ok := err_.(*exec.ExitError); ok {
    if strings.Contains(string(err.Stderr), fmt.Sprintf("No rule to make target '%s'.", target)) {
      return false, nil
    } else {
      return false, errors.New(string(err.Stderr))
    }
  } else {
    return false, err_
  }
}

func SetupMakeArgs(dir string, force bool, dryRun bool) ([]string, error) {
  if os.Getenv("MAKELEVEL") != "" {
    return nil, errors.New("can't be called inside make")
  }

  cmdArgs := make([]string, 0)

  pwd, err := os.Getwd()
  if err != nil {
    return nil, err
  }

  if pwd != dir {
    cmdArgs = append(cmdArgs, "-C", dir)
  }

  if force {
    cmdArgs = append(cmdArgs, "-B")

    if err := os.Setenv("BAKE_FORCE", "true"); err != nil {
      return nil, err
    }
  }

  if dryRun {
    //cmdArgs = append(cmdArgs, "-n")

    if err := os.Setenv("BAKE_DRYRUN", "true"); err != nil {
      return nil, err
    }
  }

  return cmdArgs, nil
}


func WriteMakefile(dir string, content string) error {
  fname := filepath.Join(dir, MAKEFILE)

  return ioutil.WriteFile(fname, []byte(content), 0644)
}
