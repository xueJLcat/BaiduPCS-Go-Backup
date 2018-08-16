package pcsweb

import (
	"github.com/iikira/BaiduPCS-Go/baidupcs"
	"github.com/iikira/BaiduPCS-Go/internal/pcsconfig"
	"io"
	"net/http"
	"fmt"
	"encoding/json"
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

func fileList(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	fpath := r.Form.Get("path")
	dataReadCloser, err := pcsconfig.Config.ActiveUserBaiduPCS().PrepareFilesDirectoriesList(fpath, baidupcs.DefaultOrderOptions)
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
