package net.quakemesh.android.ui

import android.content.Intent
import android.net.Uri
import android.text.SpannableString
import android.text.Spanned
import android.text.method.LinkMovementMethod
import android.text.style.ClickableSpan
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.android.R
import net.quakemesh.android.mesh.MeshDiscovery
import java.text.DateFormat
import java.util.Date

class PeersAdapter : RecyclerView.Adapter<PeersAdapter.Holder>() {
    private var items: List<MeshDiscovery.Peer> = emptyList()
    private val timeFmt = DateFormat.getTimeInstance(DateFormat.SHORT)

    fun submit(peers: List<MeshDiscovery.Peer>) {
        items = peers
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_peer, parent, false)
        return Holder(view)
    }

    override fun onBindViewHolder(holder: Holder, position: Int) {
        holder.bind(items[position], timeFmt)
    }

    override fun getItemCount(): Int = items.size

    class Holder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val kindView: TextView = itemView.findViewById(R.id.peer_kind)
        private val detailView: TextView = itemView.findViewById(R.id.peer_detail)
        private val seenView: TextView = itemView.findViewById(R.id.peer_seen)

        fun bind(peer: MeshDiscovery.Peer, timeFmt: DateFormat) {
            kindView.text = when (peer.kind) {
                MeshDiscovery.Kind.HUB -> itemView.context.getString(R.string.peer_kind_hub, peer.address)
                MeshDiscovery.Kind.NODE -> itemView.context.getString(R.string.peer_kind_node, peer.address)
            }
            val idShort = peer.nodeId.take(16) + if (peer.nodeId.length > 16) "…" else ""
            detailView.text = when (peer.kind) {
                MeshDiscovery.Kind.HUB -> itemView.context.getString(
                    R.string.peer_hub_detail,
                    idShort,
                    peer.heartbeatUrl ?: "",
                )
                MeshDiscovery.Kind.NODE -> {
                    val loc = if (peer.lat != null && peer.lon != null) {
                        itemView.context.getString(R.string.peer_node_location, peer.lat, peer.lon)
                    } else {
                        ""
                    }
                    itemView.context.getString(R.string.peer_node_detail, idShort, loc)
                }
            }
            seenView.text = itemView.context.getString(
                R.string.peer_last_seen,
                timeFmt.format(Date(peer.lastSeenMs)),
            )

            detailView.isClickable = false
            detailView.setOnClickListener(null)
            detailView.setBackgroundResource(0)
            if (peer.kind == MeshDiscovery.Kind.HUB && !peer.heartbeatUrl.isNullOrBlank()) {
                detailView.isClickable = true
                detailView.setBackgroundResource(android.R.drawable.list_selector_background)
                detailView.setOnClickListener {
                    showHeartbeatInfo(peer.address, peer.heartbeatUrl!!)
                }
            }
        }

        private fun showHeartbeatInfo(hubAddress: String, heartbeatUrl: String) {
            val context = itemView.context
            val monitorUrl = "http://$hubAddress:8082"
            val prefix = context.getString(R.string.heartbeat_url_info_prefix, heartbeatUrl)
            val linkLabel = context.getString(R.string.monitor_link_label)
            val suffix = context.getString(R.string.heartbeat_url_info_suffix)
            val message = SpannableString(prefix + linkLabel + suffix)
            val linkStart = prefix.length
            val linkEnd = linkStart + linkLabel.length
            message.setSpan(
                object : ClickableSpan() {
                    override fun onClick(widget: View) {
                        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(monitorUrl)))
                    }

                    override fun updateDrawState(ds: android.text.TextPaint) {
                        super.updateDrawState(ds)
                        ds.color = ContextCompat.getColor(context, android.R.color.holo_blue_dark)
                        ds.isUnderlineText = true
                    }
                },
                linkStart,
                linkEnd,
                Spanned.SPAN_EXCLUSIVE_EXCLUSIVE,
            )
            val messageView = TextView(context).apply {
                text = message
                movementMethod = LinkMovementMethod.getInstance()
                val pad = (16 * resources.displayMetrics.density).toInt()
                setPadding(pad, pad / 2, pad, 0)
            }
            AlertDialog.Builder(context)
                .setTitle(R.string.heartbeat_url_info_title)
                .setView(messageView)
                .setPositiveButton(android.R.string.ok, null)
                .show()
        }
    }
}
