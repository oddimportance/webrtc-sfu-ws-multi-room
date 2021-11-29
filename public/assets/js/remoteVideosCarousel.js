
document.addEventListener('DOMContentLoaded', function() {
    const remoteCarouselContainer = document.getElementById("remote-videos-carousel");
    // var elems = document.querySelectorAll('.carousel');
    M.Carousel.init(remoteCarouselContainer, {
            dist: 0, 
            shift: 0,
            padding: 20,
        }
    );
});
