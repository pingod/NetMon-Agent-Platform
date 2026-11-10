Randomize
Do
    Dim num
    num = Int((60 - 1 + 1) * Rnd + 1)
    Dim ret
    ret = MsgBox("随机生成的数字 (范围 1 - 60)：" & vbCrLf & vbCrLf & "          【 " & num & " 】" & vbCrLf & vbCrLf & "点击【确定】再次抽取，点击【取消】退出。", 1 + 64, "随机数生成器 (1-60)")
    If ret <> 1 Then Exit Do
Loop
