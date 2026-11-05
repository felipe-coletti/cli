package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Directory trees, processed by a pool of workers

// Job maps one input file to its output path.
type Job struct{ In, Out string }

// RunPool runs fn over jobs with n workers and returns every failure.
func RunPool(jobs []Job, n int, fn func(Job) error) []error {
	if n <= 0 {
		n = runtime.NumCPU()
	}
	n = min(n, len(jobs))

	ch := make(chan Job)
	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				if err := fn(j); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", j.In, err))
					mu.Unlock()
				}
			}
		}()
	}

	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()

	return errs
}

// PlanTree walks src and maps each regular file to a path under dst
// (dst may be empty for in-place processing).
// rename returns the output name for a file, or ok=false to skip it.
func PlanTree(src, dst string, rename func(rel string) (string, bool)) ([]Job, error) {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return nil, err
	}
	if dst != "" {
		if rel, err := filepath.Rel(srcAbs, dstAbs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, errors.New("output directory must not be inside the input directory")
		}
	}

	var jobs []Job
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			if !d.IsDir() {
				fmt.Fprintf(os.Stderr, "skip: %s (not a regular file)\n", path)
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name, ok := rename(rel)
		if !ok {
			return nil
		}
		jobs = append(jobs, Job{In: path, Out: filepath.Join(dst, name)})
		return nil
	})
	return jobs, err
}

// MkParent creates the parent directory of path.
func MkParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// ReportTree prints per-file failures and a summary; it returns an error if
// any file failed.
func ReportTree(verb string, total int, errs []error) error {
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "%s %d/%d files\n", verb, total-len(errs), total)
	if len(errs) > 0 {
		return fmt.Errorf("%d of %d files failed", len(errs), total)
	}
	return nil
}
