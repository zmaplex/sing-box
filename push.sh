#  git submodule update --init --recursive 
#  git submodule update
#  git submodule foreach git checkout main
#  git submodule foreach git pull
CGO_ENABLED=0 go build -v -trimpath -ldflags "-X 'github.com/sagernet/sing-box/constant.Version=1.9.3-pro.0' -s -w -buildid=" -tags with_dhcp,with_quic,with_ech ./cmd/sing-box
git tag test -f && git push origin test -f

