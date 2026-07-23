# 使用模板生成目录结构
```bash
go run gen/main.go -fsm ./gen/door.fsm -tpl ./gen/template
```

- fsm: 指定 .fsm 描述文件
- tpl: 指定 .tpl 模板文件所在目录
- out: 指定输出目录
-   c: 是否覆盖所有文件