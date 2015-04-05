package main

	import "github.com/fatih/color"
    import "log"
//var cyan = color.New(color.FgCyan).SprintFunc()
//var red = color.New(color.FgRed).Add(color.Bold).SprintFunc()
//var boldRed = color.New(color.FgRed).Add(color.Bold).SprintFunc()

func cyan(format string, args ...interface{}) {
	color.Set(color.FgCyan)
	log.Printf(format, args...)
	color.Unset()
}

func boldCyan(format string, args ...interface{}) {
	color.Set(color.FgCyan, color.Bold)
	log.Printf(format, args...)
	color.Unset()
}

func boldRed(format string, args ...interface{}) {
	color.Set(color.FgRed, color.Bold)
	log.Printf(format, args...)
	color.Unset()
}

func red(format string, args ...interface{}) {
	color.Set(color.FgRed)
	log.Printf(format, args...)
	color.Unset()
}

func green(format string, args ...interface{}) {
	color.Set(color.FgGreen)
	log.Printf(format, args...)
	color.Unset()
}


