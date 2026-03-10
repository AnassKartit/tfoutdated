# tfoutdated Demo Recording
# Usage: cd ~/Projects/personal/tfoutdated && vhs demo/demo.tape

Output demo/demo.gif

Set FontSize 20
Set Width 1920
Set Height 1080
Set Theme "Catppuccin Mocha"
Set Padding 24
Set TypingSpeed 40ms
Set Shell "bash"
Set Framerate 30

# ── Setup ──────────────────────────────────────────
Hide
Type `export PS1='\[\033[1;36m\]→ \[\033[0m\]'`
Enter
Sleep 200ms
Type "export TERM=xterm-256color"
Enter
Sleep 200ms
Type "export COLORTERM=truecolor"
Enter
Sleep 200ms
Type "cd /tmp/tfoutdated-demo && cp -f main.tf.bak main.tf"
Enter
Sleep 300ms
Type "clear"
Enter
Sleep 300ms
Show

# ── AWS: Scan ──────────────────────────────────────
Sleep 1s
Type "tfoutdated scan -p /tmp/tfoutdated-demo"
Enter
Sleep 12s

# ── AWS: Fix (dry run) ────────────────────────────
Type "tfoutdated fix --dry-run -p /tmp/tfoutdated-demo"
Enter
Sleep 10s

# ── AWS: Fix (apply) ──────────────────────────────
Type "tfoutdated fix -p /tmp/tfoutdated-demo"
Enter
Sleep 10s

# ── AWS: Show diff ────────────────────────────────
Type "diff main.tf.bak main.tf"
Enter
Sleep 6s

# ── Azure: Reset and scan ─────────────────────────
Type "cd /tmp/tfoutdated-demo-azure && cp -f main.tf.bak main.tf"
Enter
Sleep 500ms
Type "tfoutdated scan"
Enter
Sleep 12s

# ── Azure: Fix ────────────────────────────────────
Type "tfoutdated fix"
Enter
Sleep 10s

# ── Azure: Show diff ─────────────────────────────
Type "diff main.tf.bak main.tf"
Enter
Sleep 6s

Sleep 3s
