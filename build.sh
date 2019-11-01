#!/bin/sh

name="BaiduPCS-Go"
version=$1

GOROOT=/usr/lib/go-1.10
go=$GOROOT/bin/go

if [ "$1" = "" ];then
    version=3.6.8
fi

output="out/"

Build() {
    goarm=$4
    if [ "$4" = "" ];then
        goarm=7
    fi

    echo "Building $1..."
    export GOOS=$2 GOARCH=$3 GO386=sse2 CGO_ENABLED=0 GOARM=$4
    if [ $2 = "windows" ];then
        goversioninfo -o=resource_windows_386.syso
        goversioninfo -64 -o=resource_windows_amd64.syso
        $go build -ldflags "-X main.Version=$version -s -w" -o "$output/$1/$name.exe"
        RicePack $1 $name.exe
    else
        $go build -ldflags "-X main.Version=$version -s -w" -o "$output/$1/$name"
        RicePack $1 $name
    fi

    Pack $1
}

ArmBuild() {
    echo "Building $1..."
    export GOOS=$2 GOARCH=$3 GOARM=$4 CGO_ENABLED=1
    if [ $3 = "arm64" ];then
        CC=$ANDROID_NDK_ROOT/bin/aarch64-linux-android/bin/aarch64-linux-android-gcc $go build -ldflags "-X main.Version=$version -s -w  -linkmode=external -extldflags=-pie" -o "$output/$1/$name"
    else
        CC=$ANDROID_NDK_ROOT/bin/x86_64-linux-android/bin/x86_64-linux-android-gcc $go build -ldflags "-X main.Version=$version -s -w  -linkmode=external -extldflags=-pie" -o "$output/$1/$name"
    fi
    RicePack $1 $name
    Pack $1
}

IOSBuild() {
    echo "Building $1..."
    mkdir -p "$output/$1"
    cd "$output/$1"
    export CC=/usr/local/go/misc/ios/clangwrap.sh GOOS=darwin GOARCH=arm GOARM=7 CGO_ENABLED=1
    $go build -ldflags "-X main.Version=$version -s -w" -o "armv7" github.com/iikira/BaiduPCS-Go
    jtool --sign --inplace --ent ../../entitlements.xml "armv7"
    export GOARCH=arm64
    $go build -ldflags "-X main.Version=$version -s -w" -o "arm64" github.com/iikira/BaiduPCS-Go
    jtool --sign --inplace --ent ../../entitlements.xml "arm64"
    lipo -create "armv7" "arm64" -output $name # merge
    rm "armv7" "arm64"
    cd ../..
    RicePack $1 $name
    Pack $1
}

# zip 打包
Pack() {
    cd $output
    zip -q -r "$1.zip" "$1"

    # 删除
    rm -rf "$1"

    cd ..
}

# rice 打包静态资源
RicePack() {
    rice -i ./internal/pcsweb append --exec "$output/$1/$2"
}

# OS X / macOS
#Build $name-$version"-mac-amd64" darwin amd64

# Android
ArmBuild $name-$version"-android-arm64" android arm64 7
ArmBuild $name-$version"-android-amd64" android amd64 7

## Windows
#Build $name-$version"-windows-86" windows 386
#Build $name-$version"-windows-amd64" windows amd64
#
## Linux
#Build $name-$version"-linux-86" linux 386
#Build $name-$version"-linux-amd64" linux amd64
#Build $name-$version"-linux-arm" linux arm
#Build $name-$version"-linux-arm64" linux arm64
#GOMIPS=softfloat Build $name-$version"-linux-mipsle" linux mipsle
