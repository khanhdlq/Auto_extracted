package main

import (
	"archive/tar"
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/mholt/archiver"
	"github.com/radovskyb/watcher"
)

// func Unzip giải nén một tệp nén zip vào một thư mục đích đã chỉ định.
func Unzip(file, dest string) error {
	reader, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Create a channel to receive signals when each extraction is complete.
	done := make(chan struct{})

	// Create a counter to keep track of how many extractions are completed.
	var extractionCounter sync.WaitGroup
	extractionCounter.Add(len(reader.File))

	// Limit the number of goroutines to avoid exhausting system resources.
	concurrencyLimit := make(chan struct{}, 10) // Adjust the number as needed.

	for _, file := range reader.File {
		// Start a goroutine for each file extraction.
		go func(file *zip.File) {
			// Release the semaphore after processing.
			defer func() {
				extractionCounter.Done()
				<-concurrencyLimit
			}()

			// Acquire the semaphore to limit concurrency.
			concurrencyLimit <- struct{}{}

			filePath := filepath.Join(dest, file.Name)
			if file.FileInfo().IsDir() {
				os.MkdirAll(filePath, os.ModePerm)
				return
			}

			// Create parent directories if they don't exist.
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				log.Println("Error creating directories:", err)
				return
			}
			// Open destination file for writing.
			extractedFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
			if err != nil {
				log.Println("Error opening destination file:", err)
				return
			}
			defer extractedFile.Close()

			// Open file in zip for reading.
			zippedFile, err := file.Open()
			if err != nil {
				log.Println("Error opening zipped file:", err)
				return
			}
			defer zippedFile.Close()

			// Copy data from zipped file to extracted file.
			if _, err := io.Copy(extractedFile, zippedFile); err != nil {
				log.Println("Error copying data:", err)
				return
			}

			// Signal that this extraction is complete.
			done <- struct{}{}
		}(file)
	}

	// Wait for all extractions to be completed.
	go func() {
		extractionCounter.Wait()
		// Close the channel to signal that all extractions are done.
		close(done)
	}()

	// Wait for all extractions to be completed before returning.
	for range reader.File {
		<-done
	}

	return nil
}

// =========================================================================================================== Unzip files

type Semaphore struct {
	Wg sync.WaitGroup
	Ch chan int
}

// Limit on the number of simultaneously running goroutines.
// Depends on the number of processor cores, storage performance, amount of RAM, etc.
const grMax = 10

func saveFile(r io.Reader, target string, mode os.FileMode) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return err
	}

	return nil
}
func Untar(dst string, r io.Reader, sem *Semaphore, godeep bool) error {

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			if err := saveFile(tr, target, os.FileMode(header.Mode)); err != nil {
				return err
			}
			ext := filepath.Ext(target)

			// if it's tar file and we are on top level, extract it
			if ext == ".tar" && godeep {
				sem.Wg.Add(1)
				// A buffered channel is used to limit the number of simultaneously running goroutines
				sem.Ch <- 1
				// the file is unpacked to a directory with the file name (without extension)
				newDir := filepath.Join(dst, strings.TrimSuffix(header.Name, ".tar"))
				if err := os.Mkdir(newDir, 0755); err != nil {
					return err
				}
				go func(target string, newDir string, sem *Semaphore) {
					log.Println("start goroutine, chan length:", len(sem.Ch))
					log.Println("START:", target)
					defer sem.Wg.Done()
					defer func() { <-sem.Ch }()
					// the internal tar file opens
					ft, err := os.Open(target)
					if err != nil {
						log.Println(err)
						return
					}
					defer ft.Close()
					// the godeep parameter is false here to avoid unpacking archives inside the current archive.
					if err := Untar(newDir, ft, sem, false); err != nil {
						log.Println(err)
						return
					}
					log.Println("DONE:", target)
				}(target, newDir, sem)
			}
		}
	}
	return nil
}
func extractTar(tarFileName string, dstDir string) error {
	f, err := os.Open(tarFileName)
	if err != nil {
		return err
	}

	sem := Semaphore{}
	sem.Ch = make(chan int, grMax)

	if err := Untar(dstDir, f, &sem, true); err != nil {
		return err
	}

	sem.Wg.Wait()
	return nil
}

// =========================================================================================================== Untar files
func extract(file string, dest string) {
	// Unzip
	if strings.HasSuffix(file, ".zip") {
		if err := Unzip(file, dest); err != nil {
			log.Println("Error extracting zip:", err)
		} else {
			color.Red(file + " extracted successfully")
		}
	}

	// Use mholt/archiver for RAR and 7z extraction
	if strings.HasSuffix(file, ".rar") {
		if err := archiver.Unarchive(file, dest); err != nil {
			log.Println("Error extracting rar:", err)
		} else {
			color.Red(file + " extracted successfully")
		}
	}

	if strings.HasSuffix(file, ".7z") {
		if err := archiver.Unarchive(file, dest); err != nil {
			log.Println("Error extracting 7z:", err)
		} else {
			color.Red(file + " extracted successfully")
		}
	}

	// Untar
	if strings.HasSuffix(file, ".tar.gz") || strings.HasSuffix(file, ".tar") {
		if err := extractTar(file, dest); err != nil {
			log.Println("Error extracting tar:", err)
		} else {
			color.Red(file + " extracted successfully")
		}
	}

	//txt
	if strings.HasSuffix(file, ".txt") {
		err := os.Rename(file, dest+"/"+filepath.Base(file))
		if err != nil {
			log.Println(err)
			return
		}
	}

	// Remove the source file
	if err := os.RemoveAll(file); err != nil {
		log.Print(err)
	}

}

func walkDir(fileName string) {
	var files []string

	err := filepath.Walk(fileName, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == fileName {
			return nil
		}
		ext := filepath.Ext(path)
		allowExts := regexp.MustCompile(`(?i)\.(zip|rar|7z|tar\.gz|tar|txt)`)
		if info.IsDir() || allowExts.MatchString(ext) {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		log.Println(err)
		return
	}

	for _, file := range files {
		color.Red("[+] Processing: " + file)
		extract(file, "./extracted")
	}

}

// =========================================================================================================== Extract files
func main() {

	walkDir("./files")
	// =========================================================================================================== Walk first
	color.Green("[+] Starting watcher -> [ ./files ]")
	w := watcher.New()

	go func() {
		for {
			select {
			case event := <-w.Event:
				log.Println(event) // Print the event's info.
				// Check if the event is a create event
				if event.Op == watcher.Create {
					// Get the file info for the created file
					fileInfo, err := os.Stat(event.Path)
					if err != nil {
						log.Println(err)
						continue
					}
					// Check if it's a regular file
					if fileInfo.Mode().IsRegular() {
						// Handle here
						extract(event.Path, "./extracted")
					}
				}
			case err := <-w.Error:
				log.Println(err)
			case <-w.Closed:
				return
			}
		}
	}()
	// Watch test_folder recursively for changes.
	if err := w.AddRecursive("./files"); err != nil {
		log.Println(err)
	}
	for path, file := range w.WatchedFiles() {
		log.Printf("%s: %s\n", path, file.Name())

	}
	log.Println()

	// Trigger 2 events after watcher started.
	go func() {
		time.Sleep(time.Second)
		w.Wait()
		w.TriggerEvent(watcher.Create, nil)
		w.TriggerEvent(watcher.Remove, nil)
	}()

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 1000); err != nil {
		log.Println(err)
	}

}