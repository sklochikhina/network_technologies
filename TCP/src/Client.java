import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;

/* TODO: Клиент */
/*  1) Клиенту передаётся в параметрах относительный или абсолютный путь к файлу, который нужно отправить.
*       Длина имени файла не превышает 4096 байт в кодировке UTF-8. Размер файла не более 1 терабайта. (DONE)
*   2) Клиенту также передаётся в параметрах DNS-имя (или IP-адрес) и номер порта сервера. (DONE)
*   3) Клиент передаёт серверу имя файла в кодировке UTF-8, размер файла и его содержимое.
*       Для передачи используется TCP. Протокол передачи придумайте сами (т.е. программы разных студентов могут оказаться несовместимы).
*   4) Клиент должен вывести на экран сообщение о том, успешной ли была передача файла. */

public class Client {
    private static final int BUFFER_SIZE = 4096;
    
    public static void main(String[] args) {
        if (args.length < 3) {
            System.err.println("Usage: java TCP\\src\\Client.java <file_path> <server_ip> <server_port>");
            return;
        }
        
        String filePath =      args[0];
        String serverAddress = args[1];
        int serverPort =       Integer.parseInt(args[2]);
        
        File file = new File(filePath);
        if (!checkFile(file, filePath)) return;
        
        try (Socket socket = new Socket(serverAddress, serverPort);
             FileInputStream fis = new FileInputStream(file);
             DataOutputStream dataOutputStream = new DataOutputStream(socket.getOutputStream())) {
            
            // Отправка имени и размера файла
            dataOutputStream.writeUTF(file.getName());
            dataOutputStream.writeLong(file.length());
            
            // Отправка файла
            byte[] buffer = new byte[BUFFER_SIZE];
            int bytesRead;
            while ((bytesRead = fis.read(buffer)) > 0)
                dataOutputStream.write(buffer, 0, bytesRead);
            
            // Получение ответа от сервера
            InputStream in = socket.getInputStream();
            byte[] response = new byte[7];
            if (in.readNBytes(response, 0, 7) == -1)
                System.err.println("No response from server.");
            String result = new String(response, StandardCharsets.UTF_8);
            
            if ("SUCCESS".equals(result))
                System.out.println("File transferred successfully.");
            else
                System.err.println("Error occurred while transfer file.");
            in.close();
        } catch (IOException e) {
            System.err.println("Error: " + e.getMessage());
        }
    }
    
    private static boolean checkFile(File file, String filePath) {
        if (!file.exists()) {
            System.err.println("File does not exist: " + filePath);
            return false;
        }
        if (file.isDirectory()) {
            System.err.println("Wrong input: " + file.getAbsolutePath() + " is a directory.");
            return false;
        }
        long maxSize = (long) 1024 * 1024 * 1024 * 1024; // 1 терабайт в байтах
        if (file.length() > maxSize) {
            System.err.println("Wrong input: " + file.getAbsolutePath() + " is too large.");
            return false;
        }
        byte[] fileNameBytes = file.getName().getBytes(StandardCharsets.UTF_8);
        if (fileNameBytes.length > BUFFER_SIZE) {
            System.err.println("Wrong input: file " + file.getAbsolutePath() + " name is too long.");
            return false;
        }
        return true;
    }
}
