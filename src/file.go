package main

import (
  "io/ioutil"
  "os"
  "sort"
  "time"
)

const (
  CACHE_DIR_REL = ".cache/bake"
)

var (
  CACHE_DIR     = ""
)

type Raw struct {
  i int
  n int
  b []byte
}

type File struct {
  Path    string
  ModTime time.Time

  Head    string // whatever comes after the `//!` string on the first line
  RawDeps []string // #include "..." or import "..."
  Main    bool   // file contains a main function (we can use this to generate implicit exes)

  Deps    map[string]*File
}

func NewFile(path string, modTime time.Time, head string, rawDeps []string, main bool) *File {
  return &File{path, modTime, head, rawDeps, main, make(map[string]*File)}
}

func NewRaw(path string) (*Raw, error) {
  b, err := ioutil.ReadFile(path)
  if err != nil {
    return nil, err
  }

  return &Raw{0, len(b), b}, nil
}

func (r *Raw) NextMatch(pat []byte) (bool, bool) {
  npat := len(pat)

  if npat + r.i > r.n {
    return false, true
  }

  for i := 0; i < npat; i++ {
    if r.b[r.i + i] != pat[i] {
      return false, false
    }
  }

  r.i += npat

  return true, false
}

func (r *Raw) NextLine() bool {
  for i := r.i; i < r.n; i++ {
    if r.b[i] == '\n' || r.b[i] == '\r' {
      if i == r.n-1 {
        r.i = i
        return true
      } else {
        r.i = i+1
        return false
      }
    }
  }

  r.i = r.n-1
  return true
}

func (r *Raw) RestOfLine() (string, bool) {
  for i := r.i; i < r.n; i++ {
    if r.b[i] == '\n' || r.b[i] == '\r' {
      res := r.b[r.i:i]

      if i == r.n-1 {
        r.i = i
        return  string(res), true
      } else {
        r.i = i + 1
        return string(res), false
      }
    }
  }

  res := r.b[r.i:r.n]
  r.i = r.n-1
  return string(res), true
}

func (f *File) UniqDeps() {
  res := make(map[string]*File)

  for key, dep := range f.Deps {
    if dep != nil && dep != f {
      uniq := true
      for _, r := range res {
        if r.Path == dep.Path {
          uniq = false
          break
        }
      }

      if uniq {
        res[key] = dep
      }
    }
  }

  f.Deps = res
}

func (f *File) DstUpToDate(dst string) bool {
  isUpToDate := true
  if stat, err := os.Stat(dst); err != nil {
    isUpToDate = false
  } else {
    dstModTime := stat.ModTime()
    if f.ModTime.After(dstModTime) {
      isUpToDate = false
    } else {
      for _, dep := range f.Deps {
        if dep.ModTime.After(dstModTime) {
          isUpToDate = false
          break
        } else if !dep.DstUpToDate(dep.Path) {
          isUpToDate = false
          break
        }
      }
    }
  } 

  return isUpToDate
}

func (f *File) listDeepRawDeps(visited []*File) []string {
  for _, v := range visited {
    if v == f {
      return []string{}
    }
  }

  visited = append(visited, f)

  deps := make([]string, 0)

  for _, rawDep := range f.RawDeps {
    deps = append(deps, rawDep)
  }

  for _, dep := range f.Deps {
    deps = append(deps, dep.listDeepRawDeps(visited)...)
  }

  return deps
}

func (f *File) ListDeepRawDeps() []string {
  visited := make([]*File, 0)

  return f.listDeepRawDeps(visited)
}

type fileSorter struct {
  files []*File
}

func (fs *fileSorter) Len() int {
  return len(fs.files)
}

func (fs *fileSorter) Swap(i, j int) {
  fs.files[i], fs.files[j] = fs.files[j], fs.files[i]
}

func (fs *fileSorter) Less(i, j int) bool {
  return fs.files[i].Path < fs.files[j].Path
}

func SortUniqueFiles(files []*File) []*File {
  fs := &fileSorter{files}

  sort.Sort(fs)

  res := make([]*File, 0)

  for i, f := range fs.files {
    if i == 0 || f.Path != fs.files[i-1].Path {
      res = append(res, f)
    }
  }

  return res
}

func FilterFiles(files []*File, fn func(f *File) bool) []*File {
  res := make([]*File, 0)
  for _, f := range files {
    if fn(f) {
      res = append(res, f)
    }
  }

  return res
}
