//go:build license

package main

func appendLicensePrivacy(privacy map[string]any) {
	privacy["uploadsLicenseData"] = true
	privacy["licenseDisclosure"] = "激活时向 license.yinxiaobia.net 发送你输入的单个注册码、设备公钥、设备公钥摘要、版本号与签名请求；每分钟续租发送授权标识、设备身份、时间和递增计数器。不发送硬件序列号、电脑名、游戏账号、Riot Key 或收藏数据，不在本机保存明文注册码。设备私钥、计数器和签名租约由 Windows 当前用户 DPAPI 保护，保存在用户的 Deep Legends/license 目录；选择删除用户数据卸载时删除。服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。普通注册码由最后激活的设备使用，管理员码支持多设备；有效租约最多2小时。"
	privacy["stores"] = append(privacy["stores"].([]string), "软件授权凭据（用户目录 Deep Legends/license）：设备 Ed25519 私钥、递增计数器、服务器时间及签名租约，由 Windows 当前用户 DPAPI 保护；不保存明文注册码，选择删除用户数据卸载时删除")
}
