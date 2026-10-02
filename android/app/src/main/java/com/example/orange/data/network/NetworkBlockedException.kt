package com.example.orange.data.network

import java.io.IOException

class NetworkBlockedException :
    IOException("Network access is disabled while offline mode is active")
