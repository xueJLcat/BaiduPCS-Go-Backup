package pcsweb

import (
	"fmt"
	"os"
	"github.com/iikira/BaiduPCS-Go/internal/pcsconfig"
	"github.com/iikira/BaiduPCS-Go/baidupcs"
	"github.com/iikira/BaiduPCS-Go/pcstable"
	"github.com/iikira/BaiduPCS-Go/pcsutil/converter"
	"github.com/iikira/BaiduPCS-Go/pcsutil/waitgroup"
	"github.com/iikira/BaiduPCS-Go/requester"
	"github.com/iikira/BaiduPCS-Go/requester/downloader"
	"strconv"
	"net/url"
	"sync/atomic"
	"time"
	"io"
	"path/filepath"
	"github.com/oleiade/lane"
	"strings"
	"net/http"
	"errors"
	"github.com/iikira/BaiduPCS-Go/internal/pcscommand"
	"golang.org/x/net/websocket"
)

const (
	//DownloadSuffix 文件下载后缀
	DownloadSuffix = ".BaiduPCS-Go-downloading"
	//StrDownloadInitError 初始化下载发生错误
	StrDownloadInitError = "初始化下载发生错误"
)

// ListTask 队列状态 (基类)
type ListTask struct {
	ID       int // 任务id
	MaxRetry int // 最大重试次数
	retry    int // 任务失败的重试次数
}

// dtask 下载任务
type dtask struct {
	ListTask
	path         string                  // 下载的路径
	savePath     string                  // 保存的路径
	downloadInfo *baidupcs.FileDirectory // 文件或目录详情
}

//DownloadOptions 下载可选参数
type DownloadOptions struct {
	IsTest               bool
	IsPrintStatus        bool
	IsExecutedPermission bool
	IsOverwrite          bool
	IsShareDownload      bool
	IsLocateDownload     bool
	IsStreaming          bool
	SaveTo               string
	Parallel             int
	Load                 int
	Out                  io.Writer
}

var MsgBody string

func downloadPrintFormat(load int) string {
	if load <= 1 {
		return "\r[%d] ↓ %s/%s %s/s in %s, left %s ............"
	}
	return "[%d] ↓ %s/%s %s/s in %s, left %s ...\n"
}

func download(conn *websocket.Conn, id int, downloadURL, savePath string, loadBalansers []string, client *requester.HTTPClient, newCfg downloader.Config, downloadOptions *DownloadOptions) error {
	var (
		file     *os.File
		writerAt io.WriterAt
		err      error
		exitChan chan struct{}
	)

	if !newCfg.IsTest {
		newCfg.InstanceStatePath = savePath + DownloadSuffix

		// 创建下载的目录
		dir := filepath.Dir(savePath)
		fileInfo, err := os.Stat(dir)
		if err != nil {
			err = os.MkdirAll(dir, 0777)
			if err != nil {
				return err
			}
		} else if !fileInfo.IsDir() {
			return fmt.Errorf("%s, path %s: not a directory", StrDownloadInitError, dir)
		}

		file, err = os.OpenFile(savePath, os.O_CREATE|os.O_WRONLY, 0666)
		if file != nil {
			defer file.Close()
		}
		if err != nil {
			sendResponse(conn, 2, -4, "初始化下载发生错误", "")
		}

		// 空指针和空接口不等价
		if file != nil {
			writerAt = file
		}
	}

	download := downloader.NewDownloader(downloadURL, writerAt, &newCfg)
	download.SetClient(client)
	download.TryHTTP(!pcsconfig.Config.EnableHTTPS())
	download.AddLoadBalanceServer(loadBalansers...)

	exitChan = make(chan struct{})

	download.OnExecute(func() {
		if downloadOptions.IsPrintStatus {
			go func() {
				for {
					time.Sleep(1 * time.Second)
					select {
					case <-exitChan:
						return
					default:
						download.PrintAllWorkers()
					}
				}
			}()
		}

		if newCfg.IsTest {
			fmt.Fprintf(downloadOptions.Out, "[%d] 测试下载开始\n\n", id)
		}

		var (
			ds                            = download.GetDownloadStatusChan()
			format                        = downloadPrintFormat(downloadOptions.Load)
			downloaded, totalSize, speeds int64
			leftStr                       string
		)

		for {
			select {
			case <-exitChan:
				return
			case v, ok := <-ds:
				if !ok { // channel 已经关闭
					return
				}

				downloaded, totalSize, speeds = v.Downloaded(), v.TotalSize(), v.SpeedsPerSecond()
				if speeds <= 0 {
					leftStr = "-"
				} else {
					leftStr = (time.Duration((totalSize-downloaded)/(speeds)) * time.Second).String()
				}

				var avgSpeed int64 = 0
				timeUsed := v.TimeElapsed()/1e7*1e7
				timeSecond := v.TimeElapsed().Seconds()
				if(int64(timeSecond) > 0){
					avgSpeed = downloaded / int64(timeSecond)
				}
				//fmt.Println(timeUsed, timeSecond, avgSpeed)

				fmt.Fprintf(downloadOptions.Out, format, id,
					converter.ConvertFileSize(downloaded, 2),
					converter.ConvertFileSize(totalSize, 2),
					converter.ConvertFileSize(speeds, 2),
					timeUsed, leftStr,
				)

				MsgBody = fmt.Sprintf("{\"lastID\": %d, \"download_size\": \"%s\", \"total_size\": \"%s\", \"percent\": %.2f, \"speed\": \"%s\", \"avg_speed\": \"%s\", \"time_used\": \"%s\", \"time_left\": \"%s\"}", id,
					converter.ConvertFileSize(downloaded, 2),
					converter.ConvertFileSize(totalSize, 2),
					float64(downloaded) / float64(totalSize) * 100,
					converter.ConvertFileSize(speeds, 2),
					converter.ConvertFileSize(avgSpeed, 2),
					timeUsed, leftStr)
				err = sendResponse(conn, 2, 5, "下载中", MsgBody)
				if err != nil {
					return
				}
			}
		}
	})
	download.OnPause(func() {
		fmt.Println("\nI am paused")
	})
	download.OnResume(func() {
		fmt.Println("\nI am resume")
	})
	download.OnCancel(func() {
		fmt.Println("\nI am cancel")
	})

	err = download.Execute()
	close(exitChan)
	fmt.Fprintf(downloadOptions.Out, "\n")
	if err != nil {
		// 下载失败, 删去空文件
		if info, infoErr := file.Stat(); infoErr == nil {
			if info.Size() == 0 {
				os.Remove(savePath)
			}
		}
		return err
	}

	if downloadOptions.IsExecutedPermission {
		err = file.Chmod(0766)
		if err != nil {
			sendResponse(conn, 2, -5, "警告, 加执行权限错误", "")
			fmt.Fprintf(downloadOptions.Out, "[%d] 警告, 加执行权限错误: %s\n", id, err)
		}
	}

	if !newCfg.IsTest {
		MsgBody = fmt.Sprintf("{\"lastID\": %d, \"savePath\": \"%s\"}", id, savePath)
		err = sendResponse(conn, 2, 6, "下载完成", MsgBody)
		if err != nil {
			return err
		}
		fmt.Fprintf(downloadOptions.Out, "[%d] 下载完成, 保存位置: %s\n", id, savePath)
	} else {
		fmt.Fprintf(downloadOptions.Out, "[%d] 测试下载结束\n", id)
	}

	return nil
}

// RunDownload 执行下载网盘内文件
func RunDownload(conn *websocket.Conn, paths []string, options *DownloadOptions) (err error) {
	if options == nil {
		options = &DownloadOptions{}
	}

	if options.Out == nil {
		options.Out = os.Stdout
	}

	if options.Load < 1 {
		options.Load = pcsconfig.Config.MaxDownloadLoad()
	}

	// 设置下载配置
	cfg := &downloader.Config{
		IsTest:    options.IsTest,
		CacheSize: pcsconfig.Config.CacheSize(),
	}

	// 设置下载最大并发量
	if options.Parallel < 1 {
		options.Parallel = pcsconfig.Config.MaxParallel()
	}

	cfg.MaxParallel = pcsconfig.AverageParallel(options.Parallel, options.Load)

	paths, err = pcscommand.GetAllAbsPaths(paths...)
	if err != nil {
		fmt.Println(err)
		return
	}

	var (
		pcs    = pcsconfig.Config.ActiveUserBaiduPCS()
		dlist  = lane.NewDeque()
		lastID = 0
	)


	for k := range paths {
		lastID++
		ptask := &dtask{
			ListTask: ListTask{
				ID:       lastID,
				MaxRetry: 3,
			},
			path: paths[k],
		}
		if options.SaveTo != "" {
			ptask.savePath = filepath.Join(options.SaveTo, filepath.Base(paths[k]))
		} else {
			ptask.savePath = pcsconfig.Config.ActiveUser().GetSavePath(paths[k])
		}
		dlist.Append(ptask)

		MsgBody = fmt.Sprintf("{\"lastID\": %d, \"path\": \"%s\"}", lastID, paths[k])
		err = sendResponse(conn, 2, 1, "添加进任务队列", MsgBody)
		if err != nil {
			return err
		}
	}

	var (
		totalSize     int64
		failedList    []string
		handleTaskErr = func(task *dtask, errManifest string, err error) {
			if task == nil {
				panic("task is nil")
			}

			if err == nil {
				return
			}

			// 不重试的情况
			switch {
			case strings.Compare(errManifest, "下载文件错误") == 0 && strings.Contains(err.Error(), StrDownloadInitError):
				MsgBody = fmt.Sprintf("{\"lastID\": %d, \"errManifest\": \"%s\", \"error\": \"%s\"}", task.ID, errManifest, err)
				err = sendResponse(conn, 2, -1, "下载文件错误", MsgBody)
				return
			}
			MsgBody = fmt.Sprintf("{\"lastID\": %d, \"errManifest\": \"%s\", \"error\": \"%s\", \"retry\": %d, \"max_retry\": %d}", task.ID, errManifest, err, task.retry, task.MaxRetry)
			err = sendResponse(conn, 2, -2, "重试", MsgBody)

			// 未达到失败重试最大次数, 将任务推送到队列末尾
			if task.retry < task.MaxRetry {
				task.retry++
				dlist.Append(task)
			} else {
				failedList = append(failedList, task.path)
			}
			time.Sleep(3 * time.Duration(task.retry) * time.Second)
		}
		startTime = time.Now()
		wg        = waitgroup.NewWaitGroup(options.Load)
	)

	for {
		e := dlist.Shift()
		if e == nil { // 任务为空
			if wg.Parallel() == 0 { // 结束
				break
			} else {
				time.Sleep(1e9)
				continue
			}
		}

		task := e.(*dtask)
		wg.AddDelta()
		go func() {
			defer wg.Done()

			if task.downloadInfo == nil {
				task.downloadInfo, err = pcs.FilesDirectoriesMeta(task.path)
				if err != nil {
					// 不重试 获取路径信息错误
					MsgBody = fmt.Sprintf("{\"lastID\": %d, \"error\": \"%s\"}", task.ID, err)
					err = sendResponse(conn, 2, -3, "获取路径信息错误", MsgBody)
					return
				}
			}

			// 如果是一个目录, 将子文件和子目录加入队列
			if task.downloadInfo.Isdir {
				if !options.IsTest { // 测试下载, 不建立空目录
					os.MkdirAll(task.savePath, 0777) // 首先在本地创建目录, 保证空目录也能被保存
				}

				fileList, err := pcs.FilesDirectoriesList(task.path, baidupcs.DefaultOrderOptions)
				if err != nil {
					// 不重试 获取目录信息错误
					MsgBody = fmt.Sprintf("{\"lastID\": %d, \"error\": \"%s\"}", task.ID, err)
					sendResponse(conn, 2, -3, "获取目录信息错误", MsgBody)
					return
				}

				MsgBody = fmt.Sprintf("{\"lastID\": %d, \"path\": \"%s\"}", task.ID, task.path)
				sendResponse(conn, 2, 2, "将会使用实际的文件替代目录", MsgBody)

				for k := range fileList {
					lastID++
					subTask := &dtask{
						ListTask: ListTask{
							ID:       lastID,
							MaxRetry: 3,
						},
						path:         fileList[k].Path,
						downloadInfo: fileList[k],
					}

					if options.SaveTo != "" {
						subTask.savePath = filepath.Join(task.savePath, fileList[k].Filename)
					} else {
						subTask.savePath = pcsconfig.Config.ActiveUser().GetSavePath(subTask.path)
					}

					dlist.Append(subTask)

					MsgBody = fmt.Sprintf("{\"lastID\": %d, \"path\": \"%s\"}", lastID, fileList[k].Path)
					err := sendResponse(conn, 2, 1, "添加进任务队列", MsgBody)
					if err != nil {
						return
					}
				}
				return
			}

			MsgBody = fmt.Sprintf("{\"lastID\": %d, \"path\": \"%s\"}", task.ID, task.path)
			err := sendResponse(conn, 2, 3, "准备下载", MsgBody)
			if err != nil {
				return
			}

			if !options.IsTest && !options.IsOverwrite && fileExist(task.savePath) {
				MsgBody = fmt.Sprintf("{\"lastID\": %d, \"savePath\": \"%s\"}", task.ID, task.savePath)
				sendResponse(conn, 2, -4, "文件已经存在", MsgBody)
				return
			}

			if !options.IsTest {
				MsgBody = fmt.Sprintf("{\"lastID\": %d, \"savePath\": \"%s\"}", task.ID, task.savePath)
				err := sendResponse(conn, 2, 4, "将会下载到路径", MsgBody)
				if err != nil {
					return
				}
			}

			// 获取直链, 或者以分享文件的方式获取下载链接来下载
			var (
				dlink  string
				dlinks []string
			)

			if options.IsLocateDownload {
				// 提取直链下载
				rawDlinks := getDownloadLinks(task.path)
				if len(rawDlinks) > 0 {
					dlink = rawDlinks[0].String()
					dlinks = make([]string, 0, len(rawDlinks)-1)
					for _, rawDlink := range rawDlinks[1:len(rawDlinks)] {
						if rawDlink == nil {
							continue
						}

						dlinks = append(dlinks, rawDlink.String())
					}
				}
			} else if options.IsShareDownload {
				// 分享下载
				dlink = pcscommand.GetShareDLink(task.path)
			}

			if dlink != "" {
				client := pcsconfig.Config.HTTPClient()
				client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
					// 去掉 Referer
					if !pcsconfig.Config.EnableHTTPS() {
						req.Header.Del("Referer")
					}
					if len(via) >= 10 {
						return errors.New("stopped after 10 redirects")
					}
					return nil
				}
				client.SetTimeout(20 * time.Minute)
				client.SetKeepAlive(true)
				err = download(conn, task.ID, dlink, task.savePath, dlinks, client, *cfg, options)
			} else {
				dfunc := func(downloadURL string, jar http.CookieJar) error {
					h := pcsconfig.Config.HTTPClient()
					h.SetCookiejar(jar)
					h.SetKeepAlive(true)
					h.SetTimeout(10 * time.Minute)

					err := download(conn, task.ID, downloadURL, task.savePath, dlinks, h, *cfg, options)
					if err != nil {
						return err
					}

					return nil
				}
				if options.IsStreaming {
					err = pcs.DownloadStreamFile(task.path, dfunc)
				} else {
					err = pcs.DownloadFile(task.path, dfunc)
				}
			}

			if err != nil {
				handleTaskErr(task, "下载文件错误", err)
				return
			}

			atomic.AddInt64(&totalSize, task.downloadInfo.Size)
		}()
	}
	wg.Wait()

	fmt.Fprintf(options.Out, "\n任务结束, 时间: %s, 数据总量: %s\n", time.Since(startTime)/1e6*1e6, converter.ConvertFileSize(totalSize))
	if len(failedList) != 0 {
		fmt.Printf("以下文件下载失败: \n")
		tb := pcstable.NewTable(os.Stdout)
		for k := range failedList {
			tb.Append([]string{strconv.Itoa(k), failedList[k]})
		}
		tb.Render()
	}
	return
}

func sendResponse(conn *websocket.Conn, rtype int, rstatus int, msg string, data string) (err error){
	response := &Response{
		Code: 0,
		Type: rtype,
		Status: rstatus,
		Msg: msg,
		Data: data, //json.RawMessage(data),
	}
	if err = websocket.Message.Send(conn, string(response.JSON())); err != nil {
		fmt.Println("send err:", err.Error())
		return err
	}
	return nil
}

func getDownloadLinks(pcspath string) (dlinks []*url.URL) {
	pcs := pcsconfig.Config.ActiveUserBaiduPCS()
	dInfo, pcsError := pcs.LocateDownload(pcspath)
	if pcsError != nil {
		return
	}

	us := dInfo.URLStrings(pcsconfig.Config.EnableHTTPS())
	if len(us) == 0 {
		return
	}

	return us
}

// fileExist 检查文件是否存在,
// 只有当文件存在, 文件大小不为0或断点续传文件不存在时, 才判断为存在
func fileExist(path string) bool {
	if info, err := os.Stat(path); err == nil {
		if info.Size() == 0 {
			return false
		}
		if _, err = os.Stat(path + DownloadSuffix); err != nil {
			return true
		}
	}

	return false
}

