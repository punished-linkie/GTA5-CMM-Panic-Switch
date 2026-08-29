
# GTA5 CMM Panic Switch


## How to build
```
go mod tidy
fyne package -os windows --appID com.heistkillswitch --exe $PWD/HeistKillSwitch.exe --src src --icon $PWD\assets\icon.png -release
```