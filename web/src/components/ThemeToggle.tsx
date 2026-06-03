import { Moon, Sun } from "lucide-react"

import { Button } from "@/components/ui/button"
import { useTheme } from "@/lib/theme"

export function ThemeToggle() {
  const { resolved, setTheme } = useTheme()
  const goingDark = resolved !== "dark"

  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={goingDark ? "切换到暗色模式" : "切换到亮色模式"}
      title={goingDark ? "切换到暗色模式" : "切换到亮色模式"}
      onClick={() => setTheme(goingDark ? "dark" : "light")}
    >
      {resolved === "dark" ? <Sun /> : <Moon />}
    </Button>
  )
}
