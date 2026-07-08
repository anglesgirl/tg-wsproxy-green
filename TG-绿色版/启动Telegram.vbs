' ============================================
'  TG 绿色版静默启动器
'  双击运行:无黑窗口,后台启动代理+Telegram
' ============================================
Set WshShell = CreateObject("WScript.Shell")
Set FSO = CreateObject("Scripting.FileSystemObject")

' 获取当前脚本所在目录
strRoot = FSO.GetParentFolderName(WScript.ScriptFullName)
strProxy = strRoot & "\wsproxy\TgWsProxy.exe"
strTG = strRoot & "\Telegram\Telegram.exe"
strBat = strRoot & "\启动Telegram.bat"

' 检查文件
If Not FSO.FileExists(strProxy) Then
    MsgBox "找不到代理程序:" & vbCrLf & strProxy & vbCrLf & vbCrLf & _
           "请把 tg-ws-proxy 的 exe 放到 wsproxy 文件夹", _
           vbCritical, "TG 绿色版"
    WScript.Quit 1
End If

If Not FSO.FileExists(strTG) Then
    MsgBox "找不到 Telegram:" & vbCrLf & strTG & vbCrLf & vbCrLf & _
           "请把 Telegram Desktop 放到 Telegram 文件夹", _
           vbCritical, "TG 绿色版"
    WScript.Quit 1
End If

' 静默运行启动批处理(隐藏窗口)
WshShell.Run """" & strBat & """", 0, False

Set WshShell = Nothing
Set FSO = Nothing
