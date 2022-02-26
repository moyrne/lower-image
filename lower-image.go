package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/image/bmp"
)

var (
	dir     string
	rootCmd = &cobra.Command{
		Use: "lower-image",
		Run: execute,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&dir, "dir", "d", ".", "文件夹")

	l, err := os.Create("lower-image.log")
	if err != nil {
		panic(err)
	}

	log.SetOutput(io.MultiWriter(l, os.Stdout))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func execute(_ *cobra.Command, _ []string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	numCPU := runtime.NumCPU() - 1
	if numCPU < 1 {
		numCPU = 1
	}
	limit := make(chan struct{}, numCPU)
	wg := sync.WaitGroup{}
	for _, file := range files {
		file := file
		wg.Add(1)
		limit <- struct{}{}
		go func() {
			defer func() {
				<-limit
				wg.Done()
			}()
			src := path.Join(dir, file.Name())
			ext := filepath.Ext(src)
			switch ext {
			case ".jpg", ".jpeg", ".png", ".bmp":
			default:
				return
			}

			if err := resetFile(src); err != nil {
				log.Println("file failed", src)
				panic(err)
			}
			log.Println("file success", src)
		}()
	}
	wg.Wait()
}

func resetFile(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return errors.Wrapf(err, "读取图片信息失败: %s", filename)
	}

	rt, err := resizeImage(data)
	if err != nil {
		return errors.Wrapf(err, "压缩图片失败: %s", filename)
	}

	if err = os.MkdirAll(filepath.Join(filepath.Dir(filename), "out"), 0666); err != nil {
		return errors.Wrap(err, "创建导出文件夹失败")
	}
	newPath := filepath.Join(filepath.Dir(filename), "out", filepath.Base(filename))

	lower, err := os.Create(newPath)
	if err != nil {
		return errors.Wrapf(err, "创建压缩图片失败: %s", filename)
	}

	defer lower.Close()

	if _, err := lower.Write(rt); err != nil {
		return errors.Wrapf(err, "写入压缩图片信息失败: %s", filename)
	}

	return nil
}

const (
	threshold  = 2048 * 1024
	subPercent = 0.02
)

func resizeImage(data []byte) (rt []byte, err error) {
	img, layout, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	size := img.Bounds().Size()

	x := float64(size.X)
	y := float64(size.Y)
	length := len(data)

	per := threshold / float64(length)

	var buffer *bytes.Buffer

	var count int
	for length > threshold {
		buffer = bytes.NewBuffer(nil)
		p := math.Sqrt(per)
		resizeImg := resize.Resize(uint(x*p), uint(y*p), img, resize.Lanczos3)
		switch layout {
		case "png":
			err = png.Encode(buffer, resizeImg)
		case "jpeg", "jpg":
			err = jpeg.Encode(buffer, resizeImg, &jpeg.Options{Quality: 95})
		case "bmp":
			err = bmp.Encode(buffer, resizeImg)
		default:
			return nil, errors.Wrap(errors.New("此图片类型不支持压缩"), layout)
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}

		length = buffer.Len()
		per -= subPercent
		img = resizeImg
		count++
	}

	return buffer.Bytes(), nil
}
