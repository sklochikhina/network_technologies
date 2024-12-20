import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.time.Duration;
import java.time.Instant;
import java.util.concurrent.*;

/* TODO: Сервер */
/*  1) Cерверу передаётся в параметрах номер порта, на котором он будет ждать входящих соединений от клиентов.
*   2) Сервер сохраняет полученный файл в поддиректорию uploads своей текущей директории. Имя файла, по возможности,
*       совпадает с именем, которое передал клиент. Сервер никогда не должен писать за пределы директории uploads.
*   3) В процессе приёма данных от клиента, сервер должен раз в 3 секунды выводить в консоль мгновенную скорость приёма
*       и среднюю скорость за сеанс. Скорости выводятся отдельно для каждого активного клиента. Если клиент был активен
*       менее 3 секунд, скорость всё равно должна быть выведена для него один раз. Под скоростью здесь подразумевается
*       количество байтов переданных за единицу времени.
*   4) После успешного сохранения всего файла сервер проверяет, совпадает ли размер полученных данных с размером,
*       переданным клиентом, и сообщает клиенту об успехе или неуспехе операции, после чего закрывает соединение.
*   5) Сервер должен уметь работать параллельно с несколькими клиентами. Для этого необходимо использовать треды.
*       Сразу после приёма соединения от одного клиента, сервер ожидает следующих клиентов.
*   6) В случае ошибки сервер должен разорвать соединение с клиентом. При этом он должен продолжить обслуживать остальных клиентов.
*   7) Все используемые ресурсы ОС должны быть корректно освобождены, как только они больше не нужны. */

public class Server {
    private static final int BUFFER_SIZE = 4096;
    private static final String UPLOAD_DIR = "./TCP/src/uploads";
    
    public static void main(String[] args) throws IOException {
        if (args.length != 1) {
            System.err.println("Usage: java TCP\\src\\Server.java <port>");
            return;
        }
        
        int port = Integer.parseInt(args[0]);
        
        Path uploadDir = Paths.get(UPLOAD_DIR);
        if (!Files.exists(uploadDir)) Files.createDirectory(uploadDir);
        
        try (ServerSocket serverSocket = new ServerSocket(port)) {
            InetAddress address = serverSocket.getInetAddress();
            
            System.out.println("Server runs at ip-address " + address.getHostAddress());
            System.out.println("Server runs at port " + port);
            System.out.println("Waiting for clients...\n");
            
            ExecutorService executor = Executors.newCachedThreadPool();
            
            while (true) {
                Socket clientSocket = serverSocket.accept();
                System.out.println("New client connection from " + clientSocket.getRemoteSocketAddress());
                executor.execute(new ClientHandler(clientSocket));
            }
        }
    }
    
    static class ClientHandler implements Runnable {
        private final Socket clientSocket;
        
        ClientHandler(Socket clientSocket) {
            this.clientSocket = clientSocket;
        }
        
        @Override
        public void run() {
            try (InputStream in = clientSocket.getInputStream();
                 DataInputStream dataInputStream = new DataInputStream(in);
                 clientSocket
            ) {
                
                String fileName = dataInputStream.readUTF();
                long fileSize =   dataInputStream.readLong();
                File file = new File(fileName);
                
                Path filePath = Paths.get(UPLOAD_DIR, file.getName());
                long startTime, totalBytesReceived;
                
                try (FileOutputStream fos = new FileOutputStream(filePath.toFile())) {
                    byte[] buffer = new byte[BUFFER_SIZE];
                    totalBytesReceived = 0;
                    
                    startTime = Instant.now().toEpochMilli();
                    long lastReportTime = startTime;
                    long bytesReceivedLastInterval = 0;
                    
                    while (totalBytesReceived < fileSize) {
                        int bytesRead = dataInputStream.read(buffer);
                        fos.write(buffer, 0, bytesRead);
                        totalBytesReceived += bytesRead;
                        bytesReceivedLastInterval += bytesRead;
                        
                        long currentTime = Instant.now().toEpochMilli();
                        if (currentTime - lastReportTime >= Duration.ofSeconds(3).toMillis()) {
                            double instantaneousSpeed = bytesReceivedLastInterval / ((currentTime - lastReportTime) / 1000.0);
                            double averageSpeed =       totalBytesReceived        / ((currentTime - startTime) / 1000.0);
                            
                            System.out.printf("[Client %s] Instantaneous speed: %.2f B/s, Average speed: %.2f B/s%n",
                                    clientSocket.getRemoteSocketAddress(), instantaneousSpeed, averageSpeed);
                            
                            lastReportTime = currentTime;
                            bytesReceivedLastInterval = 0;
                        }
                    }
                }
                
                double totalTime = (Instant.now().toEpochMilli() - startTime) / 1000.0;
                double averageSpeed = totalBytesReceived / totalTime;
                System.out.printf("[Client %s] Average speed: %.2f B/s%n", clientSocket.getRemoteSocketAddress(), averageSpeed);
                
                if (totalBytesReceived == fileSize) {
                    System.out.println("File \"" + fileName + "\" successfully received from " + clientSocket.getRemoteSocketAddress() + "\n");
                    clientSocket.getOutputStream().write("SUCCESS".getBytes(StandardCharsets.UTF_8));
                } else {
                    System.err.println("Error receiving \"" + fileName + "\" from " + clientSocket.getRemoteSocketAddress() +
                                        ". Expected: " + fileSize + ", received: " + totalBytesReceived + "\n");
                    clientSocket.getOutputStream().write("FAILURE".getBytes(StandardCharsets.UTF_8));
                }
            } catch (IOException e) {
                System.err.println("Error occurred while processing the client: " + e.getMessage());
            }
        }
    }
}