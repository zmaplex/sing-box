# 2404:c140
curl -s https://android.chat.openai.com/cdn-cgi/trace -v --socks5 127.0.0.1:1080

if ! curl -s https://android.chat.openai.com/cdn-cgi/trace --socks5 127.0.0.1:1080 | grep -q "2404:c140"; then
    echo "openai is locked"
    exit 1
fi
echo "openai is unlocked"
