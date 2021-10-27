package v1

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/piupuer/go-helper/pkg/req"
	"github.com/piupuer/go-helper/pkg/resp"
	"github.com/piupuer/go-helper/pkg/utils"
	"github.com/siddontang/go/ioutil2"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func UploadUnZip(options ...func(*Options)) gin.HandlerFunc {
	ops := ParseOptions(options...)
	return func(c *gin.Context) {
		var r req.FilePartInfoReq
		req.ShouldBind(c, &r)
		r.SaveDir = ops.uploadSaveDir
		r.SingleMaxSize = ops.uploadSingleMaxSize
		if strings.TrimSpace(r.Filename) == "" {
			resp.CheckErr("filename is empty")
		}
		pwd := utils.GetWorkDir()
		fileDir, filename := filepath.Split(r.Filename)
		baseDir := fmt.Sprintf("%s/%s", pwd, fileDir)
		fullName := fmt.Sprintf("%s%s", baseDir, filename)
		unzipFiles, err := utils.UnZip(fullName, baseDir)
		if err != nil {
			resp.CheckErr(err)
		}
		// hide absolute path for front end
		files := make([]string, 0)
		for _, file := range unzipFiles {
			files = append(files, strings.TrimPrefix(file, pwd))
		}
		var rp resp.UploadUnZipResp
		rp.Files = files
		resp.SuccessWithData(files)
	}
}

func UploadFileChunkExists(options ...func(*Options)) gin.HandlerFunc {
	ops := ParseOptions(options...)
	return func(c *gin.Context) {
		var r req.FilePartInfoReq
		req.ShouldBind(c, &r)
		r.SaveDir = ops.uploadSaveDir
		r.SingleMaxSize = ops.uploadSingleMaxSize
		err := r.ValidateReq()
		resp.CheckErr(err)
		r.Complete, r.Uploaded = findUploadedChunkNumber(r)
		resp.SuccessWithData(r)
	}
}

func UploadMerge(options ...func(*Options)) gin.HandlerFunc {
	ops := ParseOptions(options...)
	return func(c *gin.Context) {
		var r req.FilePartInfoReq
		req.ShouldBind(c, &r)
		r.SaveDir = ops.uploadSaveDir
		r.SingleMaxSize = ops.uploadSingleMaxSize
		// get upload root path
		rootDir := r.GetUploadRootPath()
		mergeFileName := fmt.Sprintf("%s/%s", rootDir, r.Filename)
		mergeFile, err := os.OpenFile(mergeFileName, os.O_CREATE|os.O_WRONLY, os.ModePerm)
		resp.CheckErr(err)
		defer mergeFile.Close()

		totalChunk := int(r.GetTotalChunk())
		chunkSize := int(r.ChunkSize)
		var chunkNumbers []int
		for i := 0; i < totalChunk; i++ {
			chunkNumbers = append(chunkNumbers, i+1)
		}

		// start goroutine concurrency merge file
		var count = ops.uploadMergeConcurrentCount
		chunkCount := len(chunkNumbers) / count
		// last chunk = remainder
		lastChunkCount := chunkCount
		if len(chunkNumbers)%count > 0 || count == 1 {
			lastChunkCount = len(chunkNumbers)%count + chunkCount
		}
		chunks := make([][]int, count)
		for i := 0; i < count; i++ {
			if i < count-1 {
				chunks[i] = chunkNumbers[i*chunkCount : (i+1)*chunkCount]
			} else {
				chunks[i] = chunkNumbers[i*chunkCount : i*chunkCount+lastChunkCount]
			}
		}
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func(arr []int) {
				defer wg.Done()
				for _, item := range arr {
					func() {
						currentChunkName := r.GetChunkFilename(uint(item))
						exists := ioutil2.FileExists(currentChunkName)
						if exists {
							f, err := os.OpenFile(currentChunkName, os.O_RDONLY, os.ModePerm)
							resp.CheckErr(err)
							defer func() {
								f.Close()
							}()
							b, err := ioutil.ReadAll(f)
							resp.CheckErr(err)
							mergeFile.WriteAt(b, int64((item-1)*chunkSize))
						}
					}()
				}
			}(chunks[i])
		}
		// wait goroutine until all processing is completed
		wg.Wait()

		previewUrl := "no preview"
		if ops.uploadMinio != nil && ops.uploadMinioBucket != "" {
			// send to minio
			err = ops.uploadMinio.PutLocal(c, ops.uploadMinioBucket, mergeFileName, mergeFileName)
			if err != nil {
				resp.CheckErr("put object to minio failed, %v", err)
			}
			previewUrl = ops.uploadMinio.GetPreviewUrl(c, ops.uploadMinioBucket, mergeFileName)
		}
		// remove all chunk files
		os.RemoveAll(r.GetChunkRootPath())

		var res resp.UploadMergeResp
		res.Filename = mergeFileName
		res.PreviewUrl = previewUrl
		resp.SuccessWithData(res)
	}
}

func UploadFile(options ...func(*Options)) gin.HandlerFunc {
	ops := ParseOptions(options...)
	return func(c *gin.Context) {
		// limit file maximum memory( << 20 = 1MB)
		err := c.Request.ParseMultipartForm(ops.uploadSingleMaxSize << 20)
		if err != nil {
			resp.CheckErr("the file size exceeds the maximum: %dMB", ops.uploadSingleMaxSize)
		}
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			resp.CheckErr(err)
		}

		// read file part
		var filePart req.FilePartInfoReq
		filePart.SaveDir = ops.uploadSaveDir
		filePart.SingleMaxSize = ops.uploadSingleMaxSize
		currentSize := uint(header.Size)
		filePart.CurrentSize = &currentSize
		filePart.ChunkNumber = utils.Str2Uint(strings.TrimSpace(c.Request.FormValue("chunkNumber")))
		filePart.ChunkSize = utils.Str2Uint(strings.TrimSpace(c.Request.FormValue("chunkSize")))
		filePart.TotalSize = utils.Str2Uint(strings.TrimSpace(c.Request.FormValue("totalSize")))
		filePart.Identifier = strings.TrimSpace(c.Request.FormValue("identifier"))
		filePart.Filename = strings.TrimSpace(c.Request.FormValue("filename"))

		err = filePart.ValidateReq()
		resp.CheckErr(err)

		chunkName := filePart.GetChunkFilename(filePart.ChunkNumber)
		chunkDir, _ := filepath.Split(chunkName)
		err = os.MkdirAll(chunkDir, os.ModePerm)
		resp.CheckErr(err)

		out, err := os.Create(chunkName)
		resp.CheckErr(err)
		defer out.Close()

		_, err = io.Copy(out, file)
		resp.CheckErr(err)

		filePart.CurrentCheckChunkNumber = 1
		filePart.Complete = checkChunkComplete(filePart)
		resp.SuccessWithData(filePart)
	}
}

// check file is complete
func checkChunkComplete(filePart req.FilePartInfoReq) bool {
	currentChunkName := filePart.GetChunkFilename(filePart.CurrentCheckChunkNumber)
	exists := ioutil2.FileExists(currentChunkName)
	if exists {
		filePart.CurrentCheckChunkNumber++
		if filePart.CurrentCheckChunkNumber > filePart.GetTotalChunk() {
			return true
		}
		return checkChunkComplete(filePart)
	}
	return false
}

// find uploaded chunk files number array
func findUploadedChunkNumber(filePart req.FilePartInfoReq) (bool, []uint) {
	totalChunk := filePart.GetTotalChunk()
	var currentChunkNumber uint = 1
	uploadedChunkNumbers := make([]uint, 0)
	for {
		currentChunkName := filePart.GetChunkFilename(currentChunkNumber)
		exists := ioutil2.FileExists(currentChunkName)
		if exists {
			uploadedChunkNumbers = append(uploadedChunkNumbers, currentChunkNumber)
		}
		currentChunkNumber++
		if currentChunkNumber > totalChunk {
			break
		}
	}
	return len(uploadedChunkNumbers) == int(totalChunk), uploadedChunkNumbers
}