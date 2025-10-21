<?php

if ($_POST['mode'] == 'r') {
    // --- Configuration ---
// The target TCP server's host address.
    $target_host = '127.0.0.1';

    // The target TCP server's port.
    $target_port = 13338;

    // The connection timeout in seconds.
    $connection_timeout = 30;

    // --- Script Logic ---

    $result = [];
    $result['code'] = 0;
    $result['error'] = '';
    $result['data'] = '';

    $socket = @fsockopen("tcp://" . $target_host, $target_port, $errno, $errstr, $connection_timeout);
    if (!$socket) {
        // If the connection failed, send a "Bad Gateway" HTTP status code.
        $result['code'] = 502;
        $result['error'] = "Could not connect to the TCP server at {$target_host}:{$target_port}. Error ({$errno}): {$errstr}";
        $result['data'] = '';
    }
    $response_data = '';
    // Loop and read data from the socket in chunks until the server
// closes the connection (end-of-file is reached).
    while (!feof($socket)) {
        // fread is non-blocking in this context after the stream is opened.
        // We read in chunks of 8192 bytes for efficiency.
        $response_data .= fread($socket, 8192);
    }

    // Close the TCP socket connection as we are done with it.
    fclose($socket);

    $result['code'] = 200;
    $result['data'] = base64_encode($response_data);
    echo json_encode($result);
} elseif ($_POST['mode'] == 'w') {
    // --- Configuration ---
// The target TCP server's host address.
    $target_host = '127.0.0.1';

    // The target TCP server's port.
    $target_port = 13339;

    // The connection timeout in seconds.
    $connection_timeout = 30;

    // --- Script Logic ---

    $result = [];
    $result['code'] = 0;
    $result['error'] = '';

    if (!$_POST['data']) {
        $result['code'] = 400;
        $result['error'] = "No data provided to send to the TCP server.";
        echo json_encode($result);
        exit;
    }

    $socket = @fsockopen("tcp://" . $target_host, $target_port, $errno, $errstr, $connection_timeout);
    if (!$socket) {
        // If the connection failed, send a "Bad Gateway" HTTP status code.
        $result['code'] = 502;
        $result['error'] = "Could not connect to the TCP server at {$target_host}:{$target_port}. Error ({$errno}): {$errstr}";
    } else {

        $data_to_send = base64_decode($_POST['data']);
        $length = strlen($data_to_send);
        $bytes_sent = 0;
        while ($bytes_sent < $length) {
            $sent = fwrite($socket, substr($data_to_send, $bytes_sent), $length - $bytes_sent);
            if ($sent === false) {
                break;
            }
            $bytes_sent += $sent;
        }
        // Close the TCP socket connection as we are done with it.
        fclose($socket);

        $result['code'] = 200;

    }
    echo json_encode($result);
} else {
    $result = [];
    $result['code'] = 400;
    $result['error'] = "Invalid mode specified.";
    echo json_encode($result);
}