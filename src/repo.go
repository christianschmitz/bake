package main

import (
  "encoding/base64"
  "errors"
  "fmt"
  "io/ioutil"
  "os"
  "os/exec"
  "path/filepath"
  "regexp"
  "sort"
  "strconv"
  "strings"
  "time"
)

type FileType int

const (
  REGULAR FileType = iota
  EXE_ENTRY
  LIB_HEAD
  LIB_PART
  PCH_HEAD
)

type File struct {
  path    string
  modTime time.Time

  ft         FileType
  targetName string

  deps map[string]*File // key is the original include path
}

type Dir struct {
  path string

  dirs  []*Dir
  files []*File
}

func NewFile(path string, modTime time.Time) *File {
  return &File{path, modTime, REGULAR, "", make(map[string]*File)}
}

func NewDir(path string) *Dir {
  return &Dir{path, []*Dir{}, []*File{}}
}

func ReadDirsAndFiles(root string) (*Dir, error) {
  d := NewDir(root)

  infos, err := ioutil.ReadDir(root)
  if err != nil {
    return nil, err
  }

  for _, info := range infos {
    path := filepath.Join(root, info.Name())
    if info.IsDir() {
      sub, err := ReadDirsAndFiles(path)
      if err != nil {
        return nil, err
      }

      d.dirs = append(d.dirs, sub)
    } else {
      ext := filepath.Ext(path)
      if ext == ".cpp" || ext == ".hpp" {
        f := NewFile(path, info.ModTime())

        d.files = append(d.files, f)
      }
    }
  }

  return d, nil
}

func (d *Dir) WalkFiles(fn func(f *File) error) error {
  for _, f := range d.files {
    if err := fn(f); err != nil {
      return err
    }
  }

  for _, sub := range d.dirs {
    if err := sub.WalkFiles(fn); err != nil {
      return err
    }
  }

  return nil
}

// returns empty string if not found
func (d *Dir) FindFile(relPath string) *File {
  absPath := relPath
  if !filepath.IsAbs(relPath) {
    absPath = filepath.Join(d.path, relPath)
  }

  for _, f := range d.files {
    if absPath == f.path {
      return f
    }
  }

  for _, sub := range d.dirs {
    res := sub.FindFile(relPath)
    if res != nil {
      return res
    }
  }

  return nil
}

func (f *File) ReadDeps(root *Dir) error {
	b, err := ioutil.ReadFile(f.path)
	if err != nil {
    return err
	}

  s := string(b)

  reHead := regexp.MustCompile(`^[/][/][!](.*)`)

  msHead := reHead.FindAllStringSubmatch(s, -1)
  if len(msHead) > 0 {
    fields := strings.Fields(msHead[0][1])
    switch fields[0] {
    case "exe":
      f.ft = EXE_ENTRY
      if len(fields) > 1 {
        f.targetName = fields[1]
      }
    case "lib":
      if f.IsHPP() {
        f.ft = LIB_HEAD
      } else {
        f.ft = LIB_PART
      }

      if len(fields) > 1 {
        f.targetName = fields[1]
      }
    default:
      return errors.New(fields[0] + " is invalid build macro (see " + f.path + ")")
    }
  }

  reInclude := regexp.MustCompile(`(?m)^[#]include[\ ]["](.*?)["]`)

  ms := reInclude.FindAllStringSubmatch(s, -1)

  for _, m := range ms {
    includePath := m[1]
    if strings.HasPrefix(includePath, ".") {
      includePath = filepath.Join(filepath.Dir(f.path), includePath)
    }

    dep := root.FindFile(includePath)

    if dep != nil {
      f.deps[includePath] = dep
    }
  }

  return nil
}

func (d *Dir) ReadDeps(root *Dir) error {
  if err := d.WalkFiles(func(f *File) error {
    return f.ReadDeps(root)
  }); err != nil {
    return err
  }

  return nil
}

func (f *File) ObjPath() string {
  return filepath.Join(CacheDir(), base64.StdEncoding.EncodeToString([]byte(f.path)))
}

func (f *File) ListIncludeDirs() []string {
  includeDirs := make([]string, 0)

  thisDir := filepath.Dir(f.path)
  for key, dep := range f.deps {
    if strings.HasPrefix(key, ".") || strings.HasPrefix(key, "/") {
      continue
    }

    includeDir := dep.path[0:len(dep.path)-len(key)]

    if includeDir != thisDir {
      includeDirs = append(includeDirs, includeDir)
    }
  }

  return sortAndUniq(includeDirs)
}

func printCommandInfo(cmdName string, args []string) {
  var b strings.Builder
  b.WriteString(cmdName)

  cacheFileCount := 0

  printCacheFileInfo := func() {
    b.WriteString(" ")
    if cacheFileCount == 1 {
      b.WriteString("<obj>")
    } else {
      b.WriteString("<")
      b.WriteString(strconv.Itoa(cacheFileCount))
      b.WriteString("-objs>")
    }
    cacheFileCount = 0
  }

  for _, arg := range args {
    if strings.HasPrefix(arg, CACHE_DIR) {
      cacheFileCount += 1
    } else {
      if cacheFileCount != 0 {
        printCacheFileInfo()
      }

      b.WriteString(" ")
      if strings.HasPrefix(arg, ROOT_PATH) {
        b.WriteString(".")
        b.WriteString(strings.TrimPrefix(arg, ROOT_PATH))
      } else {
        b.WriteString(arg)
      }
    }
  }

  if cacheFileCount != 0 {
    printCacheFileInfo()
  }

  fmt.Println(b.String())
}

func runCommand(cmdName string, args []string) error {
  cmd := exec.Command(cmdName, args...)

  printCommandInfo(cmdName, args)

  cmd.Stdout = os.Stdout
  cmd.Stdin = os.Stdin
  cmd.Stderr = os.Stderr

  return cmd.Run()
}

func (f *File) ObjUpToDate() bool {
  objPath := f.ObjPath()

  isUpToDate := true
  if stat, err := os.Stat(objPath); err != nil || FORCE {
    isUpToDate = false
  } else {
    objModTime := stat.ModTime()
    if f.modTime.After(objModTime) || CONFIG_MOD_TIME.After(objModTime) {
      isUpToDate = false
    } else {
      for _, dep := range f.deps {
        if dep.modTime.After(objModTime) {
          isUpToDate = false
          break
        }
      }
    }
  } 

  return isUpToDate
}

func (f *File) Compile(cfg *Config) error {
  objPath := f.ObjPath()

  cmdArgs := []string{}
  includeDirs := f.ListIncludeDirs()

  for _, id := range includeDirs {
    cmdArgs = append(cmdArgs, "-I")
    cmdArgs = append(cmdArgs, id)
  }

  cmdArgs = append(cmdArgs, "-c", f.path, "-o", objPath)

  SetUpdatedObj(objPath)

  return runCommand(cfg.Compiler, cmdArgs)
}

func sortAndUniq(strs []string) []string {
  sort.Strings(strs)

  uniq := make([]string, 0)
  for i := 0; i < len(strs); i++ {
    if i == 0 || strs[i] != strs[i-1] {
      uniq = append(uniq, strs[i])
    }
  }

  return uniq
}

func (f *File) IsHPP() bool {
  return strings.HasSuffix(f.path, ".hpp")
}

func (f *File) IsCPP() bool {
  return strings.HasSuffix(f.path, ".cpp")
}

func (f *File) CollectExeObjs(root *Dir) []string {
  dirs := []string{}

  // any cpp files in same directory
  // any cpp files in directories of dependencies

  for _, dep := range f.deps {
    dirs = append(dirs, filepath.Dir(dep.path))
  }

  dirs = sortAndUniq(dirs)

  objs := []string{f.ObjPath()}

  inDir := func(path string) bool {
    for _, d := range dirs {
      if d == filepath.Dir(path) {
        return true
      }
    }

    return false
  }

  if err := root.WalkFiles(func(f *File) error {
    if f.IsCPP() && inDir(f.path) {
      objs = append(objs, f.ObjPath())
    }
    
    return nil
  }); err != nil {
    panic(err)
  }

  return sortAndUniq(objs)
}

func (f *File) ExePath(dstDir string) string {
  name := f.targetName
  if name == "" {
    name = filepath.Base(filepath.Dir(f.path))
  }

  dst := filepath.Join(dstDir, name)

  return dst
}

func (f *File) ExeUpToDate(dstDir string, root *Dir) bool {
  dst := f.ExePath(dstDir)

  objs := f.CollectExeObjs(root)

  isUpToDate := true
  if stat, err := os.Stat(dst); err != nil || FORCE {
    isUpToDate = false
  } else {
    if CONFIG_MOD_TIME.After(stat.ModTime()) {
      isUpToDate = false
    } else {
      for _, obj := range objs {
        if IsUpdatedObj(obj) {
          isUpToDate = false
          break
        }
      }
    }
  }

  return isUpToDate
}

func (f *File) LinkExe(cfg *Config, dstDir string, root *Dir) error {
  dst := f.ExePath(dstDir)

  // TODO: reuse Objs from ExeUpToDate()
  objs := f.CollectExeObjs(root)

  cmdArgs := strings.Fields(cfg.ExeFlags)
  cmdArgs = append(cmdArgs, "-o", dst)
  cmdArgs = append(cmdArgs, objs...)

  SetUpdatedExe(dst)

  return runCommand(cfg.Compiler, cmdArgs)
}
