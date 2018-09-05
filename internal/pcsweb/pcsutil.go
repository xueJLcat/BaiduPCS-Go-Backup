package pcsweb

import (
	"github.com/iikira/BaiduPCS-Go/baidupcs"
	"github.com/iikira/BaiduPCS-Go/internal/pcsconfig"
	"io"
	"net/http"
	"fmt"
	"encoding/json"
	"github.com/iikira/BaiduPCS-Go/internal/pcscommand"
	"strings"
	"strconv"
	"io/ioutil"
	"BaiduPCS-Go/pcsutil/converter"
)

type Response struct {
	Code int         `json:"code"`
	Type int         `json:"type"`
	Status int       `json:"status"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type pcsConfigJSON struct {
	Name string `json:"name"`
	EnName  string `json:"en_name"`
	Value string `json:"value"`
	Desc string `json:"desc"`
}

func (res *Response) JSON() (data []byte) {
	var err error
	data, err = json.Marshal(res)
	checkErr(err)
	return
}

func UserHandle(w http.ResponseWriter, r *http.Request) {
	activeUser := pcsconfig.Config.ActiveUser()
	response := &Response{
		Code: 0,
		Data: activeUser,
	}
	w.Write(response.JSON())
}

func QuotaHandle(w http.ResponseWriter, r *http.Request) {
	quota, used, _ := pcsconfig.Config.ActiveUserBaiduPCS().QuotaInfo()
	quotaMsg := fmt.Sprintf("{\"quota\": \"%s\", \"used\": \"%s\", \"un_used\": \"%s\", \"percent\": %.2f}",
		converter.ConvertFileSize(quota, 2),
		converter.ConvertFileSize(used, 2),
		converter.ConvertFileSize(quota - used, 2),
		100 * float64(used) / float64(quota))
	response := &Response{
		Code: 0,
		Data: quotaMsg,
	}
	w.Write(response.JSON())
}

func DownloadHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	method := r.Form.Get("method")
	id, _ := strconv.Atoi(r.Form.Get("id"))

	dl := DownloaderMap[id]
	if dl == nil {
		response := &Response{
			Code: -6,
			Msg: "任务已经终结",
		}
		w.Write(response.JSON())
		return
	}

	response := &Response{
		Code: 0,
		Msg: "success",
	}
	switch method {
	case "pause":
		dl.Pause()
	case "resume":
		dl.Resume()
	case "cancel":
		dl.Cancel()
	case "status":
		response.Data = dl.GetAllWorkersStatus()
	}
	w.Write(response.JSON())
}

func ShareHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rmethod := r.Form.Get("method")
	if rmethod == "list" {
		records, err := pcsconfig.Config.ActiveUserBaiduPCS().ShareList(1)
		if err != nil {
			response := &Response{
				Code: -1,
				Msg: err.Error(),
			}
			w.Write(response.JSON())
			return
		}
		response := &Response{
			Code: 0,
			Data: records,
		}
		w.Write(response.JSON())
	}
	if rmethod == "cancel" {
		rids := strings.Split(r.Form.Get("id"), ",")
		ids := make([]int64, 0, 10)
		for _, sid := range rids {
			tmp, _ := strconv.Atoi(sid)
			ids = append(ids, int64(tmp))
		}
		err := pcsconfig.Config.ActiveUserBaiduPCS().ShareCancel(ids)
		if err != nil {
			response := &Response{
				Code: -1,
				Msg: err.Error(),
			}
			w.Write(response.JSON())
			return
		}
		response := &Response{
			Code: 0,
			Msg: "success",
		}
		w.Write(response.JSON())
	}
	if rmethod == "set" {
		rpath := r.Form.Get("paths")
		rpaths := strings.Split(rpath, "|")
		paths := make([]string, 0, 10)
		for _, path := range rpaths {
			paths = append(paths, path)
		}
		fmt.Println(rpath, paths)
		shared, err := pcsconfig.Config.ActiveUserBaiduPCS().ShareSet(paths, nil)
		if err != nil {
			response := &Response{
				Code: -1,
				Msg: err.Error(),
			}
			w.Write(response.JSON())
			return
		}
		response := &Response{
			Code: 0,
			Msg: shared.Link,
		}
		w.Write(response.JSON())
	}
}

func SettingHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rmethod := r.Form.Get("method")
	config := pcsconfig.Config
	if rmethod == "get" {
		configJsons := make([]pcsConfigJSON, 0, 10)
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "PCS应用ID",
			EnName: "appid",
			Value: strconv.Itoa(config.AppID()),
			Desc: "",
		})
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "启用 https",
			EnName: "enable_https",
			Value: fmt.Sprint(config.EnableHTTPS()),
			Desc: "",
		})
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "浏览器标识",
			EnName: "user_agent",
			Value: config.UserAgent(),
			Desc: "",
		})
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "下载缓存",
			EnName: "cache_size",
			Value: strconv.Itoa(config.CacheSize()),
			Desc: "建议1024 ~ 262144, 如果硬盘占用高或下载速度慢, 请尝试调大此值",
		})
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "下载最大并发量",
			EnName: "max_parallel",
			Value: strconv.Itoa(config.MaxParallel()),
			Desc: "建议50 ~ 500. 单任务下载最大线程数量",
		})
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "同时下载数量",
			EnName: "max_download_load",
			Value: strconv.Itoa(config.MaxDownloadLoad()),
			Desc: "建议 1 ~ 5, 同时进行下载文件的最大数量",
		})
		configJsons = append(configJsons, pcsConfigJSON{
			Name: "下载目录",
			EnName: "savedir",
			Value: config.SaveDir(),
			Desc: "下载文件的储存目录",
		})
		response := &Response{
			Code: 0,
			Data: configJsons,
		}
		w.Write(response.JSON())
	}
	if rmethod == "set" {
		enable_https := r.Form.Get("enable_https")
		user_agent := r.Form.Get("user_agent")
		cache_size := r.Form.Get("cache_size")
		max_parallel := r.Form.Get("max_parallel")
		max_download_load := r.Form.Get("max_download_load")
		savedir := r.Form.Get("savedir")
		_, err := ioutil.ReadDir(savedir)
		if err != nil {
			response := &Response{
				Code: -1,
				Msg: "输入的文件夹路径错误",
			}
			w.Write(response.JSON())
			return
		}

		bool_value, _ := strconv.ParseBool(enable_https)
		config.SetEnableHTTPS(bool_value)
		config.SetUserAgent(user_agent)
		int_value, _ :=strconv.Atoi(cache_size)
		config.SetCacheSize(int_value)
		int_value, _ = strconv.Atoi(max_parallel)
		config.SetMaxParallel(int_value)
		int_value, _ = strconv.Atoi(max_download_load)
		config.SetMaxDownloadLoad(int_value)
		config.SetSaveDir(savedir)
		config.Save()
	}
}

func LogoutHandle(w http.ResponseWriter, r *http.Request) {
	activeUser := pcsconfig.Config.ActiveUser()
	deletedUser, err := pcsconfig.Config.DeleteUser(&pcsconfig.BaiduBase{
		UID: activeUser.UID,
	})
	if err != nil {
		fmt.Printf("退出用户 %s, 失败, 错误: %s\n", activeUser.Name, err)
	}

	fmt.Printf("退出用户成功, %s\n", deletedUser.Name)
	err = pcsconfig.Config.Save()
	if err != nil {
		fmt.Printf("保存配置错误: %s\n", err)
	}
	fmt.Printf("保存配置成功\n")
}

func LocalFileHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rpath:= r.Form.Get("path")
	files, err := ListLocalDir(rpath, "")
	if err != nil {
		w.Write((&Response{
			Code: -1,
			Msg:  err.Error(),
		}).JSON())
		return
	}
	w.Write((&Response{
		Code: 0,
		Data: files,
	}).JSON())
}

func FileOperationHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rmethod := r.Form.Get("method")
	rpaths:= r.Form.Get("paths")
	paths := strings.Split(rpaths, "|")
	var err error
	if (rmethod == "copy"){
		err = pcscommand.RunCopy(paths...)
	} else if (rmethod == "move"){
		err = pcscommand.RunMove(paths...)
	} else if (rmethod == "remove"){
		err = pcscommand.RunRemove(paths...)
	} else {
		w.Write((&Response{
			Code: -2,
			Msg:  "method error",
		}).JSON())
	}
	if err != nil {
		w.Write((&Response{
			Code: -1,
			Msg:  err.Error(),
		}).JSON())
		return
	}
	w.Write((&Response{
		Code: 0,
		Msg:  "success",
	}).JSON())
}

func MkdirHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rpath := r.Form.Get("path")
	err := pcscommand.RunMkdir(rpath)
	if err != nil {
		w.Write((&Response{
			Code: 1,
			Msg:  err.Error(),
		}).JSON())
		return
	}
	w.Write((&Response{
		Code: 0,
		Msg:  "success",
	}).JSON())
}

func fileList(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	fpath := r.Form.Get("path")
	orderBy := r.Form.Get("order_by")
	order := r.Form.Get("order")
	orderOptions := &baidupcs.OrderOptions{}
	switch {
	case order == "asc":
		orderOptions.Order = baidupcs.OrderAsc
	case order == "desc":
		orderOptions.Order = baidupcs.OrderDesc
	default:
		orderOptions.Order = baidupcs.OrderAsc
	}

	switch {
	case orderBy == "time":
		orderOptions.By = baidupcs.OrderByTime
	case orderBy == "name":
		orderOptions.By = baidupcs.OrderByName
	case orderBy == "size":
		orderOptions.By = baidupcs.OrderBySize
	default:
		orderOptions.By = baidupcs.OrderByName
	}

	dataReadCloser, err := pcsconfig.Config.ActiveUserBaiduPCS().PrepareFilesDirectoriesList(fpath, orderOptions)

	w.Header().Set("content-type", "application/json")

	if err != nil {
		w.Write((&Response{
			Code: 1,
			Msg:  err.Error(),
		}).JSON())
		return
	}

	defer dataReadCloser.Close()
	io.Copy(w, dataReadCloser)
}
