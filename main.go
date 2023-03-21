package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-commp-utils/writer"
	"github.com/filecoin-project/go-fil-markets/stores"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/unixfs"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/gin-gonic/gin"
	"github.com/ipfs/go-cidutil/cidenc"
	"github.com/ipld/go-car"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/multiformats/go-multibase"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	IPFS_API = "http://127.0.0.1:5001"
	IPFS_GATEWAY="http://127.0.0.1:8080/ipfs"
)

type FileType int64

const (
	OriginFile  FileType =iota+1
	CarFile
)
func main() {
	r := gin.Default()
	r.POST("/upload", uploadHandler)
	r.Run(":8080")
}

type Response struct {
	CID         string `json:"cid"`
	Link        string `json:"link"`
	CarLink     string `json:"car_link"`
	PayloadCID  string `json:"payload_cid"`
	CommPCID    string `json:"comm_p_cid"`
	PieceSize   int64  `json:"piece_size"`
	CarFileSize int64  `json:"car_file_size"`
}

func uploadHandler(c *gin.Context) {
	// 配置 logrus
	log := logrus.New()
	log.Out = ioutil.Discard
	log.Formatter = &logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	}

	file, err := c.FormFile("file")
	if err != nil {
		log.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to read file",
		})
		return
	}
	tmpfile := fmt.Sprintf("nft_src_%d", time.Now().UnixNano())
	err = c.SaveUploadedFile(file, tmpfile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.RemoveAll(tmpfile)
	resp := Response{}

	//generatecar
	carpath := fmt.Sprintf("%s.car", tmpfile)
	if err := generatecar(tmpfile, carpath, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := computeComp(carpath, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := uploadToIpfs(carpath,CarFile, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := uploadToIpfs(tmpfile, OriginFile, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.RemoveAll(carpath)
	c.JSON(http.StatusOK, resp)
}

func generatecar(inPath string, outPath string, response *Response) error {
	randStr := "tmpfile"               // 随机字符串
	timestamp := time.Now().UnixNano() // 当前时间戳（纳秒）
	tempFile := randStr + fmt.Sprintf("_%d", timestamp)
	// generate and import the UnixFS DAG into a filestore (positional reference) CAR.
	root, err := unixfs.CreateFilestore(context.TODO(), inPath, tempFile)
	if err != nil {
		return fmt.Errorf("failed to import file using unixfs: %w", err)
	}

	// open the positional reference CAR as a filestore.
	fs, err := stores.ReadOnlyFilestore(tempFile)
	if err != nil {
		return fmt.Errorf("failed to open filestore from carv2 in path %s: %w", tempFile, err)
	}
	defer fs.Close() //nolint:errcheck

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}

	// build a dense deterministic CAR (dense = containing filled leaves)
	if err := car.NewSelectiveCar(
		context.TODO(),
		fs,
		[]car.Dag{{
			Root:     root,
			Selector: selectorparse.CommonSelector_ExploreAllRecursively,
		}},
		car.MaxTraversalLinks(config.MaxTraversalLinks),
	).Write(
		f,
	); err != nil {
		return fmt.Errorf("failed to write CAR to output file: %w", err)
	}

	err = f.Close()
	if err != nil {
		return err
	}
	encoder := cidenc.Encoder{Base: multibase.MustNewEncoder(multibase.Base32)}
	response.PayloadCID = encoder.Encode(root)
	if err:=os.RemoveAll(tempFile);err!=nil{
		return err
	}
	return nil
}

func uploadToIpfs(src string, fileType FileType,response *Response) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("", "")
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	err = writer.Close()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/v0/add", IPFS_API)
	req, err := http.NewRequest("POST", url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err != nil {
		return err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return err
	}
	if response==nil{
		return nil
	}
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Body must be split into individual json objects since what is returned now is not a valid json object
	bodyParts := strings.Split(string(responseBody), "\n")
	type ipfsAddResponse struct {
		Name string `json:"Name"`
		Hash string `json:"Hash"`
		Size string `json:"Size"`
	}
	// The second to last object in this list is the pinned folder
	var folderResponse ipfsAddResponse
	err = json.Unmarshal([]byte(bodyParts[len(bodyParts)-2]), &folderResponse)
	if err != nil {
		return err
	}
	if fileType==CarFile{
		response.CarLink=fmt.Sprintf("%s/%s",IPFS_GATEWAY, folderResponse.Hash)
	}else {
		response.Link = fmt.Sprintf("%s/%s",IPFS_GATEWAY, folderResponse.Hash)
		response.CID=folderResponse.Hash
	}
	return nil
}

func computeComp(inPath string, response *Response) error {
	rdr, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer rdr.Close() //nolint:errcheck

	w := &writer.Writer{}
	_, err = io.CopyBuffer(w, rdr, make([]byte, writer.CommPBuf))
	if err != nil {
		return fmt.Errorf("copy into commp writer: %w", err)
	}

	commp, err := w.Sum()
	if err != nil {
		return fmt.Errorf("computing commP failed: %w", err)
	}

	encoder := cidenc.Encoder{Base: multibase.MustNewEncoder(multibase.Base32)}

	stat, err := os.Stat(inPath)
	if err != nil {
		return err
	}
	response.CommPCID = encoder.Encode(commp.PieceCID)
	response.PieceSize = types.NewInt(uint64(commp.PieceSize.Unpadded().Padded())).Int64()
	response.CarFileSize = stat.Size()
	return nil
}
