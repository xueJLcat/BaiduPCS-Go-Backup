package pcsweb

import (
	"golang.org/x/net/websocket"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/iikira/BaiduPCS-Go/internal/pcsconfig"
	"github.com/iikira/Baidu-Login"
	"github.com/iikira/BaiduPCS-Go/internal/pcscommand"
)


func getValueFromWSJson(conn *websocket.Conn, key string) (resString string, err error) {
	var reply string
	if err = websocket.Message.Receive(conn, &reply); err != nil {
		fmt.Println("receive err:", err.Error())
		return
	}
	rJson, err := simplejson.NewJson([]byte(reply))
	if err != nil {
		fmt.Println("receive err:", err.Error())
		return
	}

	resString, _ = rJson.Get(key).String()
	return
}

func WSLogin(conn *websocket.Conn, rJson *simplejson.Json) (err error) {
	vcode := ""
	vcodestr := ""

	username, _ := rJson.Get("username").String()
	password, _ := rJson.Get("password").String()
	fmt.Println(username, password)

	bc := baidulogin.NewBaiduClinet()
	var BDUSS, PToken, SToken string
	for i := 0; i < 10; i++ {
		lj := bc.BaiduLogin(username, password, vcode, vcodestr)
		fmt.Println(lj)

		switch lj.ErrInfo.No {
		case "0": // 登录成功, 退出循环
			BDUSS, PToken, SToken = lj.Data.BDUSS, lj.Data.PToken, lj.Data.SToken
			goto loginSuccess
		case "400023", "400101": // 需要验证手机或邮箱
			verifyTypes := fmt.Sprintf("[{\"label\": \"mobile %s\", \"value\": \"mobile\"}, {\"label\": \"email %s\", \"value\": \"email\"}]", lj.Data.Phone, lj.Data.Email)
			response := &Response{
				Code: 0,
				Type: 1,
				Status: 2,
				Data: verifyTypes,
			}
			if err = websocket.Message.Send(conn, string(response.JSON())); err != nil {
				fmt.Println("send err:", err.Error())
				return err
			}

			verifyType, err := getValueFromWSJson(conn, "verify_type")
			if err != nil {
				fmt.Println("receive err:",err.Error())
				return err
			}
			fmt.Println(verifyType)

			msg := bc.SendCodeToUser(verifyType, lj.Data.Token) // 发送验证码
			if err = websocket.Message.Send(conn, "{\"code\": 0, \"type\": 1, \"status\": 3}"); err != nil {
				fmt.Println("send err:", err.Error())
				return err
			}
			fmt.Printf("消息: %s\n\n", msg)

			for et := 0; et < 5; et++ {
				vcode, err = getValueFromWSJson(conn, "verify_code")
				if err != nil {
					fmt.Println("receive err:",err.Error())
					return err
				}
				fmt.Println(vcode)

				nlj := bc.VerifyCode(verifyType, lj.Data.Token, vcode, lj.Data.U)
				if nlj.ErrInfo.No != "0" {
					errMsg := fmt.Sprintf("{\"code\": 0, \"type\": 1, \"status\": 4, \"error_time\":%d, \"error_msg\":\"%s\"}", et+1, nlj.ErrInfo.Msg)
					if err = websocket.Message.Send(conn, errMsg); err != nil {
						fmt.Println("send err:", err.Error())
						return err
					}
					continue
				}
				// 登录成功
				BDUSS, PToken, SToken = nlj.Data.BDUSS, nlj.Data.PToken, nlj.Data.SToken
				goto loginSuccess
			}
		case "400038": //账号密码错误
			errMsg := fmt.Sprintf("{\"code\": 0, \"type\": 1, \"status\": 5, \"msg\":\"account or password error\"}")
			if err = websocket.Message.Send(conn, errMsg); err != nil {
				fmt.Println("send err:", err.Error())
				return err
			}
			return err
		case "500001", "500002": // 验证码
			fmt.Printf("\n%s\n", lj.ErrInfo.Msg)
			if lj.ErrInfo.No == "500002"{
				errMsg := fmt.Sprintf("{\"code\": 0, \"type\": 1, \"status\": 4}")
				if err = websocket.Message.Send(conn, errMsg); err != nil {
					fmt.Println("send err:", err.Error())
					return err
				}
			}
			vcodestr = lj.Data.CodeString
			if vcodestr == "" {
				err = fmt.Errorf("未找到codeString")
				return err
			}

			verifyImgURL := "https://wappass.baidu.com/cgi-bin/genimage?" + vcodestr

			errMsg := fmt.Sprintf("{\"code\": 0, \"type\": 1, \"status\": 6, \"img_url\":\"%s\"}", verifyImgURL)
			if err = websocket.Message.Send(conn, errMsg); err != nil {
				fmt.Println("send err:", err.Error())
				return err
			}

			vcode, err = getValueFromWSJson(conn, "verify_code")
			if err != nil {
				fmt.Println("receive err:",err.Error())
				return err
			}
			fmt.Println(vcode)
			continue
		default:
			err = fmt.Errorf("错误代码: %s, 消息: %s", lj.ErrInfo.No, lj.ErrInfo.Msg)
			response := &Response{
				Code: -1,
				Msg: err.Error(),
			}
			if err = websocket.Message.Send(conn, string(response.JSON())); err != nil {
				fmt.Println("send err:", err.Error())
			}
			return err
		}
	}

loginSuccess:
	fmt.Println("loginSuccess", BDUSS, PToken, SToken)
	baidu, err := pcsconfig.Config.SetupUserByBDUSS(BDUSS, PToken, SToken)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("百度帐号登录成功:", baidu.Name)
	successMsg := fmt.Sprintf("{\"code\": 0, \"type\": 1, \"status\": 7, \"username\":\"%s\"}", baidu.Name)
	if err = websocket.Message.Send(conn, successMsg); err != nil {
		fmt.Println("send err:", err.Error())
		return err
	}

	err = pcsconfig.Config.Save()
	if err != nil {
		fmt.Printf("保存配置错误: %s\n", err)
	}
	fmt.Printf("保存配置成功\n")
	return err
}

func WSDownload(conn *websocket.Conn, rJson *simplejson.Json) (err error) {
	paths, _ := rJson.Get("paths").StringArray()
	fmt.Println(paths)

	options := &DownloadOptions{
		IsTest: false,
		IsOverwrite: true,
	}

	return RunDownload(conn, paths, options)
}

func WSUpload(conn *websocket.Conn, rJson *simplejson.Json) (err error) {
	paths, _ := rJson.Get("paths").StringArray()
	tpath, _ := rJson.Get("tpath").String()
	fmt.Println(paths, tpath)

	//options := &DownloadOptions{
	//	IsTest: false,
	//	IsOverwrite: true,
	//}

	pcscommand.RunUpload(paths, tpath, nil)
	return
}

func WSHandler(conn *websocket.Conn){
	fmt.Printf("a new ws conn: %s->%s\n", conn.RemoteAddr().String(), conn.LocalAddr().String())

	for {
		var reply string
		if err := websocket.Message.Receive(conn, &reply); err != nil {
			fmt.Println("receive err:", err.Error())
			return
		}
		rJson, err := simplejson.NewJson([]byte(reply))
		if err != nil {
			fmt.Println("receive err:", err.Error())
			return
		}
		rType, _ := rJson.Get("type").Int()

		switch rType {
		case 1:
			WSLogin(conn, rJson)
			if err != nil {
				fmt.Println("WSLogin err:", err.Error())
				continue
			}
		case 2:
			WSDownload(conn, rJson)
			if err != nil {
				fmt.Println("WSDownload err:", err.Error())
				continue
			}
		case 3:
			WSUpload(conn, rJson)
			if err != nil {
				fmt.Println("WSUpload err:", err.Error())
				continue
			}
		}
	}
}

