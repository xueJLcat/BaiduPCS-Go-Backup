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
	"net/url"
)

type Response struct {
	Code int         `json:"code"`
	Type int         `json:"type"`
	Status int       `json:"status"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
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

func FileOperationHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rmethod := r.Form.Get("method")
	rpaths, _ := url.QueryUnescape(r.Form.Get("paths"))
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
			Code: 2,
			Msg:  "method error",
		}).JSON())
	}
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

func MkdirHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	rpath, _ := url.QueryUnescape(r.Form.Get("path"))
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

	fpath, _ := url.QueryUnescape(r.Form.Get("path"))
	dataReadCloser, err := pcsconfig.Config.ActiveUserBaiduPCS().PrepareFilesDirectoriesList(fpath, baidupcs.DefaultOrderOptions)

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
