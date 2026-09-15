/* ================================================================================================
 * internal/configuracao/rede.go - Enderecos de rede do VaiJunto
 * Autor: Arthur Souza
 *
 * Centraliza o endereco padrao usado pelos clientes e pelo programa de carga.
 * O IP 192.168.1.5 pertence a interface Ethernet da maquina usada nesta configuracao.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O cliente precisa conhecer o IP do computador servidor. O servidor escuta em todas as
 * interfaces para aceitar conexoes locais, pela rede e dentro de um conteiner Docker.
 * ================================================================================================ */

package configuracao

/* ServidorPadrao: destino dos clientes e do teste de carga quando nao ha outra configuracao.
 * PARA USAR OUTRO COMPUTADOR COMO SERVIDOR: troque 192.168.1.5 pelo IPv4 desse computador.
 * Descubra esse IPv4 com ipconfig no Windows ou hostname -I no Linux.
 * Outra opcao e usar -servidor IP:8080 ou VAIJUNTO_SERVIDOR, sem editar o codigo.
 * Um IP atribuido pelo roteador pode mudar ao trocar de rede ou reconectar a maquina.
 */
const ServidorPadrao = "192.168.1.5:8080"

/* EscutaPadrao: porta onde o servidor aceita conexoes em todas as interfaces locais.
 * Nao coloque o IP da maquina hospedeira aqui para executar dentro do Docker: esse IP
 * nao pertence necessariamente ao conteiner. O Compose publica a porta do conteiner no host.
 * Use -endereco ou VAIJUNTO_ENDERECO se precisar mudar a escuta na execucao com Go.
 */
const EscutaPadrao = ":8080"
