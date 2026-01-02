# fluffy-palm-tree

![screenshot](https://github.com/PeterJohnBishop/fluffy-palm-tree/blob/main/assets/screenshot.png?raw=true)

A secure, multi-room chat application using a combination of WebSockets for real-time communication and PAKE (Password-Authenticated Key Exchange) for cryptographic security.

Client and server verify they both know the same password and generate a shared session key through PAKE. 

The password is encrypted using Argon2 to generate a 32-bit encrypted roomKey.

Each client runs readPump and writePump goroutines to listen for and push data concurrently. 

The terminal user interface is built on the Charmbracelet/BubbleTea framework and displays chats optionally in an encrypted or decrypted state. 

TLDR: message > client encrypts > server broadcasts > clients decrypt > message displayed

![demo gif](https://github.com/PeterJohnBishop/fluffy-palm-tree/blob/main/assets/snipit_demo.gif?raw=true)