# 2404:c140

if ! curl -s https://android.chat.openai.com/cdn-cgi/trace --socks5 127.0.0.1:1080 | grep -Eq "2404:c140|38.150.8"; then
    echo "openai is locked"
    exit 1
fi

echo "openai is unlocked"
