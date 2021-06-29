package main

import (
  "encoding/base64"
  "errors"
  "os"
  "path/filepath"
  "strings"
  "time"
)

var (
  HEXTS  = []string{".h", ".hpp", ".hxx", ".hh"}
  CEXTS  = []string{".c", ".cpp", ".cxx", ".cc"}
  HCEXTS = append(HEXTS, CEXTS...)

  HEAD_PAT    = []byte("//!")
  INCLUDE_PAT = []byte("#include ")
  MAIN_PAT    = []byte("int main(")
)

type CProject struct {
  ProjectData

  updatedObjs    []string // also pch can go here
  compilerCmd    string
  linkerCmd      string
  emitPchCmd     string
  includePchOpts string
}

func NewCProject(args []string) (Project, error) {
  p := &CProject{}

  rem, err := p.ProjectData.InitProject(args)
  if err != nil {
    return nil, err
  }

  p.updatedObjs = make([]string, 0)

  rem, err = ParseStringFlags(rem, []string{"--compiler", "--linker", "--emit-pch", "--include-pch"}, []*string{
    &p.compilerCmd,
    &p.linkerCmd,
    &p.emitPchCmd,
    &p.includePchOpts,
  })

  if p.compilerCmd == "" {
    return nil, errors.New("--compiler not specified")
  }

  if p.linkerCmd == "" {
    return nil, errors.New("--linker not specified")
  }

  if err != nil {
    return nil, err
  }

  if err := AssertNoArgs(rem); err != nil {
    return nil, err
  }

  p.files = make([]*File, 0)

  if err := p.WalkFiles(func(path string, info os.FileInfo) error {
    if !p.IsHCFile(path) {
      return nil
    }

    f, err := p.ParseCFile(path, info.ModTime())
    if err != nil {
      return err
    }

    p.files = append(p.files, f)

    return nil
  }); err != nil {
    return nil, err
  }

  return p, nil
}

func (p *CProject) IsHCFile(path string) bool {
  return ContainsString(HCEXTS, filepath.Ext(path))
}

func (p *CProject) IsHFile(path string) bool {
  return ContainsString(HEXTS, filepath.Ext(path)) 
}

func (p *CProject) IsCFile(path string) bool {
  return ContainsString(CEXTS, filepath.Ext(path))
}

func (p *CProject) IsExeFile(f *File) bool {
  return f.Main || strings.HasPrefix(f.Head, "exe")
}

func (p *CProject) IsPchFile(f *File) bool {
  return strings.HasPrefix(f.Head, "pch")
}

func (p *CProject) ParseCFile(path string, modTime time.Time) (*File, error) {
  r, err := NewRaw(path)
  if err != nil {
    return nil, err
  }

  head := ""
  rawDeps := make([]string, 0)
  main := false

  isMatch, eof := r.NextMatch(HEAD_PAT)
  if isMatch && !eof {
    head, eof = r.RestOfLine()
  }

  for !eof {
    isMatch, eof = r.NextMatch(INCLUDE_PAT)
    if isMatch {
      var rawDep string
      rawDep, eof = r.RestOfLine()

      if rawDep[0] == '"' {
        fs := strings.Split(rawDep[1:], "\"")
        if len(fs) > 1 {
          rawDep = fs[0]
          rawDeps = append(rawDeps, rawDep)
        }
      } else if rawDep[0] == '<' {
        fs := strings.Split(rawDep[1:], ">")
        if len(fs) > 1 {
          rawDep = fs[0]
          rawDeps = append(rawDeps, "<" + rawDep + ">")
        }
      }
    } else if isMatch, eof = r.NextMatch(MAIN_PAT); isMatch {
      main = true
      eof = r.NextLine()
    } else {
      eof = r.NextLine()
    }
  }

  if main {
    if !p.IsCFile(path) {
      main = false
    }
  }

  return NewFile(path, modTime, head, rawDeps, main), nil
}

func (p *CProject) ResolveDeps() error {
  for _, f := range p.files {
    fDir := filepath.Dir(f.Path)

    for _, rawDep := range f.RawDeps {
      if filepath.IsAbs(rawDep) {
        f.Deps[rawDep] = p.FindFile(rawDep)
      } else if strings.HasPrefix(rawDep, ".") {
        f.Deps[rawDep] = p.FindFile(filepath.Join(fDir, rawDep))
      } else {
        f.Deps[rawDep] = p.FindFileBySuffix(rawDep)
      }
    }

    f.UniqDeps()
  }

  return nil
}

func (p *CProject) ObjPath(f *File) string {
  return filepath.Join(CACHE_DIR, base64.URLEncoding.EncodeToString([]byte(f.Path)))
}

func (p *CProject) PchPath(f *File) string {
  return p.ObjPath(f) + ".pch"
}

func (p *CProject) ObjUpToDate(f *File) bool {
  objPath := p.ObjPath(f)

  return f.DstUpToDate(objPath)
}

func (p *CProject) LibName(f *File) string {
  if strings.HasPrefix(f.Head, "lib") {
    fs := strings.Fields(f.Head)

    if len(fs) > 1 {
      return fs[1]
    }
  }

  return filepath.Base(filepath.Dir(f.Path))
}

// TODO: should also work on windows
func (p *CProject) LibPath(f *File) string {
  base := p.LibName(f) + ".so"

  return filepath.Join(p.dstDir, base)
}

func (p *CProject) ExeName(f *File) string {
  if strings.HasPrefix(f.Head, "exe") {
    fs := strings.Fields(f.Head)

    if len(fs) > 1 {
      return fs[1]
    }
  }

  return filepath.Base(filepath.Dir(f.Path))
}

func (p *CProject) ExePath(f *File) string {
  base := p.ExeName(f)

  return filepath.Join(p.dstDir, base)
}

func (p *CProject) ListExeObjFiles(f *File) []*File {
  dirs := make([]string, 0)

  // any cpp files in same directory
  // any cpp files in directories of dependencies

  for _, dep := range f.Deps {
    dirs = append(dirs, filepath.Dir(dep.Path))
  }

  dirs = SortUnique(dirs)

  inDir := func(path string) bool {
    for _, d := range dirs {
      if d == filepath.Dir(path) {
        return true
      }
    }

    return false
  }

  files := p.FilterFiles(func(f *File) bool {
    return p.IsCFile(f.Path) && inDir(f.Path)
  })

  // include self of course
  files = append(files, f)

  return SortUniqueFiles(files)
}

func (p *CProject) ListExeObjs(f *File) []string {
  fObjs := p.ListExeObjFiles(f)

  objPaths := make([]string, len(fObjs))
  for i, fObj := range fObjs {
    objPaths[i] = p.ObjPath(fObj)
  }

  return objPaths
}

func (p *CProject) ExeUpToDate(f *File) bool {
  dst := p.ExePath(f)

  objs := p.ListExeObjs(f)

  isUpToDate := true
  if _, err := os.Stat(dst); err != nil {
    isUpToDate = false
  } else {
    for _, obj := range objs {
      if p.IsUpdatedObj(obj) {
        isUpToDate = false
        break
      }
    }
  }

  return isUpToDate
}

func (p *CProject) getPchFile() (*File, error) {
  if p.emitPchCmd == "" || p.includePchOpts == "" {
    return nil, nil
  }

  pchFiles := p.FilterFiles(func(f *File) bool {
    return p.IsPchFile(f)
  })

  if len(pchFiles) == 0 {
    return nil, nil
  } else if len(pchFiles) == 1 {
    return pchFiles[0], nil
  } else {
    return nil, errors.New("multiple pch files found (there can only be one)")
  }
}

func (p *CProject) HasPchFile() (bool, error) {
  f, err := p.getPchFile()
  if err != nil {
    return false, err
  }

  if f == nil {
    return false, nil
  } else {
    return true, nil
  }
}

func (p *CProject) includeDirOpts(f *File) string {
  includeDirs := p.ListIncludeDirs(f)

  if len(includeDirs) == 0 {
    return ""
  } else {
    return "-I " + strings.Join(includeDirs, "-I ")
  }
}

func (p *CProject) buildPch() error {
  f, err := p.getPchFile()
  if err != nil {
    return err
  }

  if f == nil {
    return nil
  }

  pchPath := p.PchPath(f)

  if f.DstUpToDate(pchPath) && !p.force {
    return nil
  }

  templateArgs := map[string]string{
    "include": p.includeDirOpts(f),
    "header": f.Path,
    "output": pchPath,
  }

  cmdStr, err := FillTemplate(p.emitPchCmd, templateArgs, "--emit-pch")
  if err != nil {
    return err
  }

  cmdName, cmdArgs := SplitCommand(cmdStr)

  p.PrintCommand(cmdName, cmdArgs)

  if !p.dryRun {
    return RunCommand(cmdName, cmdArgs)
  } else {
    return nil
  }
}

func (p *CProject) Build() error {
  if err := p.buildPch(); err != nil {
    return err
  }

  cppFiles := p.FilterFiles(func(f *File) bool {
    return p.IsCFile(f.Path) && (p.force || !p.ObjUpToDate(f))
  })

  if err := RunPar(len(cppFiles), func(i int) error {
    return p.CompileObj(cppFiles[i])
  }); err != nil {
    return err
  }

  exeFiles := p.FilterFiles(func(f *File) bool {
    return p.IsExeFile(f) && (p.force || !p.ExeUpToDate(f))
  })

  return RunPar(len(exeFiles), func(i int) error {
    return p.CompileExe(exeFiles[i])
  })
}

func (p *CProject) BuildTarget(target string) error {
  if err := p.buildPch(); err != nil {
    return err
  }

  exeFiles := p.FilterFiles(func(f *File) bool {
    return p.IsExeFile(f) && (p.ExeName(f) == target)
  })

  if len(exeFiles) == 0 {
    return errors.New("bake target " + target + " not found")
  } else if len(exeFiles) > 1 {
    return errors.New("bake target " + target + " ambiguous")
  } 

  // TODO: compile libs

  exeFile := exeFiles[0]

  cppFiles := p.ListExeObjFiles(exeFile)

  cppFiles = FilterFiles(cppFiles, func(f *File) bool {
    return p.force || !p.ObjUpToDate(f)
  })

  if err := RunPar(len(cppFiles), func(i int) error {
    return p.CompileObj(cppFiles[i])
  }); err != nil {
    return err
  }

  return p.CompileExe(exeFile)
}

func (p *CProject) ListIncludeDirs(f *File) []string {
  includeDirs := make([]string, 0)

  thisDir := filepath.Dir(f.Path)

  for key, dep := range f.Deps {
    // abs or rel paths dont require and include path
    if strings.HasPrefix(key, ".") || strings.HasPrefix(key, "/") {
      continue
    }

    if !filepath.IsAbs(dep.Path) {
      panic("all filepaths should be abs at this point")
    }

    includeDir := strings.TrimRight(strings.TrimSuffix(dep.Path, key), "/")

    if includeDir != thisDir {
      includeDirs = append(includeDirs, includeDir)
    }
  }

  return SortUnique(includeDirs)
}

func (p *CProject) IsUpdatedObj(obj string) bool {
  p.mutex.RLock()

  defer p.mutex.RUnlock()

  return ContainsString(p.updatedObjs, obj)
}

func (p *CProject) IncludePchOpts(cmdStr string) (string, error) {
  pch, err := p.getPchFile()
  if err != nil {
    return "", err
  }

  if pch != nil {
    pchArgs := map[string]string{
      "pch": p.PchPath(pch),
    }

    pchStr, err := FillTemplate(p.includePchOpts, pchArgs, "--include-pch")
    if err != nil {
      return "", err
    }

    cmdStr += " " + pchStr
  }

  return cmdStr, nil
}

func (p *CProject) CompileObj(f *File) error {
  objPath := p.ObjPath(f)

  templateArgs := map[string]string{
    "include": p.includeDirOpts(f),
    "source": f.Path,
    "output": objPath,
  }


  cmdStr, err := FillTemplate(p.compilerCmd, templateArgs, "--compiler")
  if err != nil {
    return err
  }

  cmdStr, err = p.IncludePchOpts(cmdStr)
  if err != nil {
    return err
  }

  p.mutex.Lock()

  p.updatedObjs = append(p.updatedObjs, objPath)

  p.mutex.Unlock()

  cmdName, cmdArgs := SplitCommand(cmdStr)

  p.PrintCommand(cmdName, cmdArgs)

  if !p.dryRun {
    return RunCommand(cmdName, cmdArgs)
  } else {
    return nil
  }
}

func (p *CProject) ListExeLibs(f *File) ([]string, error) {
  deps_ := f.ListDeepRawDeps()

  // filter the system files out
  deps := make([]string, 0)
  for _, dep := range deps_ {
    if len(dep) > 2 && dep[0] == '<' && dep[len(dep)-1] == '>' {
      deps = append(deps, dep)
    }
  }

  res := make([]string, 0)

  if ContainsAnyString(deps, []string{"<math>", "<cmath>"}) {
    res = append(res, "m")
  }

  if ContainsString(deps, "OpenCL/cl2") {
    res = append(res, "OpenCL")
  }

  if ContainsAnyString(deps, []string{"<iostream>", "<string>", "<concepts>", "<utility>", "<map>", "<vector>", "<type_traits>", "<memory>", "<sstream>"}) {
    res = append(res, "stdc++")
  }

  return res, nil
}

func (p *CProject) CompileExe(f *File) error {
  dst := p.ExePath(f)

  objs := p.ListExeObjs(f)

  libs, err := p.ListExeLibs(f)
  if err != nil {
    return err
  }

  libOpts := ""
  if len(libs) > 0 {
    libOpts = "-l" + strings.Join(libs, " -l")
  }

  templateArgs := map[string]string{
    "objects": strings.Join(objs, " "),
    "output": dst,
    "libs": libOpts,
  }

  cmdStr, err := FillTemplate(p.linkerCmd, templateArgs, "--linker")
  if err != nil {
    return err
  }

  cmdName, cmdArgs := SplitCommand(cmdStr)

  p.PrintCommand(cmdName, cmdArgs)

  if !p.dryRun {
    return RunCommand(cmdName, cmdArgs)
  } else {
    return nil
  }
}

func (p *CProject) CompileLib(f *File) error {
  panic("not yet implemented")
}
