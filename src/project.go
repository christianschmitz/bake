package main

import (
  "errors"
  "io/ioutil"
  "os"
  "path/filepath"
  "regexp"
  "strings"
  "sync"
)

type Project interface {
  ResolveDeps() error

  Build() error
  BuildTarget(target string) error
}

type ProjectData struct {
  force   bool
  dryRun  bool
  root    string
  dstDir  string

  files   []*File

  mutex   *sync.RWMutex
}

func (p *ProjectData) InitProject(args []string) ([]string, error) {
  rem, err := ParseGeneralArgsDefaultDirPwd(args, &p.force, &p.dryRun, &p.root)
  if err != nil {
    return nil, err
  }

  dstDir := ""

  rem, err = ParseStringFlags(rem, []string{"--dst"}, []*string{&dstDir})
  if err != nil {
    return nil, err
  }

  if dstDir == "" {
    return nil, errors.New("--dst not specified")
  }

  if !filepath.IsAbs(dstDir) {
    dstDir = filepath.Join(p.root, dstDir)
  }

  if err := os.MkdirAll(dstDir, 0755); err != nil {
    return nil, err
  }

  p.dstDir = dstDir
  
  if p.mutex == nil {
    p.mutex = &sync.RWMutex{}
  }

  if os.Getenv("BAKE_FORCE") != "" {
    p.force = true
  }

  if os.Getenv("BAKE_DRYRUN") != "" {
    p.dryRun = true
  }

  return rem, nil
}

func WalkFiles(dir string, fn func(path string, info os.FileInfo) error) error {
  infos, err := ioutil.ReadDir(dir)
  if err != nil {
    return err
  }

  for _, info := range infos {
    path := filepath.Join(dir, info.Name())
    if info.IsDir() {
      if err := WalkFiles(path, fn); err != nil {
        return err
      }
    } else {
      if err := fn(path, info); err != nil {
        return err
      }
    }
  }

  return nil
}

func (p *ProjectData) WalkFiles(fn func(path string, info os.FileInfo) error) error {
  return WalkFiles(p.root, fn)
}

func FillTemplate(tmp string, args map[string]string, ctx string) (string, error) {
  orig := tmp

  for key, val := range args {
    key_ := "{" + key + "}"
    if !strings.Contains(tmp, key_) {
      return "", errors.New(ctx + " template doesn't contain " + key_ + "(" + orig + ")")
    }

    tmp = strings.Replace(tmp, key_, val, 1)
  }

  reRest := regexp.MustCompile(`[{][a-z]+?[}]`)

  rest := reRest.FindAllString(tmp, -1)

  if len(rest) > 0 {
    return "", errors.New("unrecognized template string variable(s) in " + ctx + ": " + strings.Join(rest, ", "))
  }

  return tmp, nil
}

func (p *ProjectData) FindFile(path string) *File {
  for _, f := range p.files {
    if f.Path == path {
      return f
    }
  }

  return nil
}

func (p *ProjectData) FindFileBySuffix(suffix string) *File {
  for _, f := range p.files {
    if strings.HasSuffix(f.Path, suffix) {
      return f
    }
  }

  return nil
}

func (p *ProjectData) FilterFiles(fn func(f *File) bool) []*File {
  return FilterFiles(p.files, fn)
}

func (p *ProjectData) PrintCommand(cmdName string, cmdArgs []string) {
  PrintCommand(p.root, CACHE_DIR, cmdName, cmdArgs)
}
